package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/merge"
	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/0xdps/daemon-hound/internal/storage"
	"github.com/0xdps/daemon-hound/internal/tracker"
	"github.com/0xdps/daemon-hound/internal/utils"
)

// Syncer orchestrates pull and push operations between local files and the vault.
type Syncer struct {
	vault           *storage.Vault
	tracker         *tracker.Tracker
	git             *git.Client
	config          ConfigReader
	namespaceFilter string // if non-empty, only sync this namespace
	merger          *merge.Registry
}

// ConfigReader provides read/write access to local config needed by the syncer.
type ConfigReader interface {
	MachineID() string
	GetBinding(namespace string) (string, bool)
	SetBinding(namespace, localPath string) error
	IsFileIgnored(namespace, relPath string) bool
	PendingPush() bool
	SetPendingPush(pending bool) error
}

// NewSyncer creates a new Syncer.
func NewSyncer(vault *storage.Vault, tracker *tracker.Tracker, gitClient *git.Client, config ConfigReader) *Syncer {
	return &Syncer{
		vault:   vault,
		tracker: tracker,
		git:     gitClient,
		config:  config,
		merger:  merge.NewRegistry(),
	}
}

// WithMerger returns a Syncer that uses the given merge registry for smart
// conflict resolution. The registry should already include a SecretDriver
// if vault identity is available.
func (s *Syncer) WithMerger(merger *merge.Registry) *Syncer {
	copy := *s
	copy.merger = merger
	return &copy
}

// WithNamespaceFilter returns a Syncer that only processes the given namespace.
func (s *Syncer) WithNamespaceFilter(ns string) *Syncer {
	copy := *s
	copy.namespaceFilter = ns
	return &copy
}

// shouldProcess reports whether a file should be included given the current filters.
func (s *Syncer) shouldProcess(file models.TrackedFile) bool {
	if s.config.IsFileIgnored(file.Namespace, file.RelPath) {
		return false
	}
	if file.Mode == models.ModeBackup && file.MachineID != s.config.MachineID() {
		return false
	}
	if s.namespaceFilter != "" && file.Namespace != s.namespaceFilter {
		return false
	}
	return true
}

// Result holds the outcome of a sync operation for a single file.
type Result struct {
	File   models.TrackedFile
	Action string // "pushed", "pulled", "skipped", "conflict"
	Error  error
}

// Pull pulls the latest vault state and restores any tracked files for known namespaces.
func (s *Syncer) Pull() ([]Result, error) {
	if err := s.git.Pull(); err != nil {
		return nil, fmt.Errorf("failed to pull vault: %w", err)
	}

	state, err := s.vault.LoadState()
	if err != nil {
		return nil, fmt.Errorf("failed to load vault state: %w", err)
	}

	var results []Result
	for _, file := range state.Files {
		if !s.shouldProcess(file) {
			continue
		}

		// For global files, only restore if this machine tracked them
		if file.Namespace == "global" {
			if file.MachineID != s.config.MachineID() {
				continue
			}
		} else {
			// For namespace files, only restore if we have a binding
			if _, ok := s.config.GetBinding(file.Namespace); !ok {
				continue
			}
		}

		// CRITICAL: If the local file has been modified since the last sync,
		// it is newer than the vault content. Do NOT overwrite it.
		// The next Push() will upload the local version.
		status, err := s.tracker.Status(file)
		if err != nil {
			results = append(results, Result{File: file, Action: "error", Error: err})
			continue
		}
		if status == models.StatusDirty || status == models.StatusNew {
			results = append(results, Result{File: file, Action: "skipped", Error: nil})
			continue
		}

		localPath, err := s.tracker.Restore(file)
		if err != nil {
			results = append(results, Result{File: file, Action: "error", Error: err})
			continue
		}

		// The remote state.toml.age already contains the authoritative checksum
		// written by the pushing machine. After Restore() the local file has
		// exactly that content, so there is nothing to write back to state.
		// We must NOT update LastSyncAt here — doing so would dirty state on
		// every pull and generate a spurious commit that bounces forever
		// between machines (A pulls → new timestamp → commits → B pulls → ...).
		_ = localPath

		results = append(results, Result{File: file, Action: "pulled"})
	}

	return results, nil
}

// Push encrypts and pushes locally modified tracked files to the vault.
func (s *Syncer) Push() ([]Result, error) {
	state, err := s.vault.LoadState()
	if err != nil {
		return nil, fmt.Errorf("failed to load vault state: %w", err)
	}

	var results []Result
	stateModified := false
	for key, file := range state.Files {
		if !s.shouldProcess(file) {
			continue
		}

		status, err := s.tracker.Status(file)
		if err != nil {
			results = append(results, Result{File: file, Action: "error", Error: err})
			continue
		}

		if status != models.StatusDirty && status != models.StatusNew {
			continue
		}

		localPath, err := s.resolveLocalPath(file)
		if err != nil {
			results = append(results, Result{File: file, Action: "error", Error: err})
			continue
		}

		plaintext, err := os.ReadFile(localPath)
		if err != nil {
			results = append(results, Result{File: file, Action: "error", Error: err})
			continue
		}

		written, err := s.vault.StoreFile(file, plaintext)
		if err != nil {
			results = append(results, Result{File: file, Action: "error", Error: err})
			continue
		}

		// If StoreFile was a no-op (content unchanged) update only the checksum
		// in state so Status() returns clean next time, but don't mark state dirty.
		checksum, err := utils.FileChecksum(localPath)
		if err != nil {
			results = append(results, Result{File: file, Action: "error", Error: err})
			continue
		}
		file.Checksum = checksum
		file.LastSyncAt = time.Now()
		state.Files[key] = file
		if written {
			stateModified = true
		}

		results = append(results, Result{File: file, Action: "pushed"})
	}

	// Only re-encrypt and write state when something actually changed.
	// SaveState always produces different ciphertext (fresh age nonce) so we
	// must guard it — an unconditional write would create a spurious git commit
	// on every sync cycle even when nothing changed.
	if stateModified {
		if err := s.vault.SaveState(state); err != nil {
			return results, fmt.Errorf("failed to save vault state: %w", err)
		}
	}

	// Commit and push
	hasChanges, err := s.git.HasChanges()
	if err != nil {
		return results, fmt.Errorf("failed to check git status: %w", err)
	}

	if hasChanges {
		if err := s.git.CommitAll("daemon-hound sync"); err != nil {
			return results, fmt.Errorf("failed to commit: %w", err)
		}
		if err := s.git.Push(); err != nil {
			// Offline — mark pending so status and the next sync know to retry.
			fmt.Fprintf(os.Stderr, "Warning: could not push to remote (offline?): %v\n", err)
			fmt.Fprintf(os.Stderr, "Changes committed locally. Run `dhd sync` again when online.\n")
			_ = s.config.SetPendingPush(true)
			return results, nil
		}
		// Successful push — clear any pending marker.
		if s.config.PendingPush() {
			_ = s.config.SetPendingPush(false)
		}
	} else if s.config.PendingPush() {
		// No new local changes but a previous push failed — retry the push.
		if err := s.git.Push(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: retry push failed (still offline?): %v\n", err)
			return results, nil
		}
		_ = s.config.SetPendingPush(false)
	}

	return results, nil
}

// Sync runs Push (commit local changes) then Pull then re-Push to remote.
// Committing local changes first ensures git pull always has a clean working
// tree to merge into — avoids the "local changes would be overwritten" abort.
func (s *Syncer) Sync() ([]Result, error) {
	// Pre-flight: if a previous merge was left unfinished (e.g. a prior sync
	// crashed mid-pull) the vault is stuck and git pull will refuse to run.
	// Vault files are encrypted binary blobs — 3-way text merge is meaningless.
	// Recover by aborting the stale merge; we will re-push local changes below.
	if s.git.IsInMerge() {
		if hasConflicts, _ := s.git.HasConflicts(); hasConflicts {
			fmt.Fprintln(os.Stderr, "Warning: recovering from stuck merge — taking remote version of conflicted files")
			if err := s.git.ResolveConflictsRemote(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: conflict resolution failed: %v\n", err)
				_ = s.git.AbortMerge() // last resort: abort and start fresh
			} else {
				_ = s.git.CommitAll("daemon-hound: resolve vault merge conflicts [auto]")
			}
		} else {
			// In-merge but no conflicted files — just commit the resolved state.
			_ = s.git.CommitAll("daemon-hound: complete merge [auto]")
		}
	}

	// Phase 1: Encrypt and commit any locally dirty files so git has a clean
	// working tree before we attempt a pull.
	pushResults, pushErr := s.Push()
	if pushErr != nil {
		return pushResults, pushErr
	}

	// Phase 2: Pull remote changes and restore any files updated on other machines.
	pullResults, pullErr := s.Pull()
	all := append(pullResults, pushResults...)
	if pullErr != nil {
		// Check if pull failed due to conflicts (remote deleted files we added).
		if hasConflicts, _ := s.git.HasConflicts(); hasConflicts {
			fmt.Fprintln(os.Stderr, "Warning: merge conflict during pull — attempting smart merge")
			if err := s.resolveConflicts(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: smart merge failed: %v — falling back to remote\n", err)
				if err := s.git.ResolveConflictsRemote(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: conflict resolution failed: %v\n", err)
					_ = s.config.SetPendingPush(true)
					return all, nil
				}
			}
			_ = s.git.CommitAll("daemon-hound: resolve vault merge conflicts [auto]")
			// Fall through to Phase 3 to push local changes on top.
		} else {
			// Offline or unreachable — local changes are already committed; try to push.
			fmt.Fprintf(os.Stderr, "Warning: could not pull from remote (offline?): %v\n", pullErr)
			if err := s.git.Push(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not push to remote (offline?): %v\n", err)
				_ = s.config.SetPendingPush(true)
			} else {
				_ = s.config.SetPendingPush(false)
			}
			return all, nil
		}
	}

	// Phase 3: Commit any state changes written by Pull (restored file checksums)
	// and push everything to remote.
	if err := s.git.CommitAll("daemon-hound: sync"); err == nil {
		if err := s.git.Push(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not push to remote (offline?): %v\n", err)
			_ = s.config.SetPendingPush(true)
		} else {
			_ = s.config.SetPendingPush(false)
		}
	}

	return all, nil
}

// DryRun shows what Sync would do without performing any reads from remote or writes to disk.
func (s *Syncer) DryRun() ([]Result, error) {
	state, err := s.vault.LoadState()
	if err != nil {
		return nil, fmt.Errorf("failed to load vault state: %w", err)
	}

	var results []Result
	for _, file := range state.Files {
		if !s.shouldProcess(file) {
			continue
		}

		fileStatus, err := s.tracker.Status(file)
		if err != nil {
			results = append(results, Result{File: file, Action: "error", Error: err})
			continue
		}

		switch fileStatus {
		case models.StatusDirty, models.StatusNew:
			results = append(results, Result{File: file, Action: "would push"})
		case models.StatusMissing:
			if s.vault.FileExistsInVault(file) {
				results = append(results, Result{File: file, Action: "would pull"})
			} else {
				results = append(results, Result{File: file, Action: "missing (not in vault)"})
			}
		default:
			results = append(results, Result{File: file, Action: "up to date"})
		}
	}
	return results, nil
}

// resolveLocalPath determines the local filesystem path for a tracked file.
func (s *Syncer) resolveLocalPath(file models.TrackedFile) (string, error) {
	if file.Namespace == "global" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s/%s", home, file.RelPath), nil
	}
	if root, ok := s.config.GetBinding(file.Namespace); ok {
		return fmt.Sprintf("%s/%s", root, file.RelPath), nil
	}

	root, err := discoverRepoRootForNamespace(file.Namespace)
	if err != nil {
		return "", fmt.Errorf("no local binding for namespace %s: %w", file.Namespace, err)
	}
	_ = s.config.SetBinding(file.Namespace, root)
	return fmt.Sprintf("%s/%s", root, file.RelPath), nil
}

func discoverRepoRootForNamespace(namespace string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	var match string
	err = filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
		if err != nil || match != "" {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" {
			repoRoot := filepath.Dir(path)
			origin, err := utils.GetGitOrigin(repoRoot)
			if err != nil {
				return nil
			}
			discovered, err := utils.DeriveNamespace(origin)
			if err != nil {
				return nil
			}
			if discovered == namespace {
				match = repoRoot
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if match == "" {
		return "", fmt.Errorf("no repo root found for namespace %s", namespace)
	}
	return match, nil
}

// resolveConflicts attempts smart merge for each conflicted file using the
// configured merge registry. Files that merge cleanly are staged automatically.
// Files with true conflicts fall back to ResolveConflictsRemote.
func (s *Syncer) resolveConflicts() error {
	conflictedFiles, err := s.git.GetConflictedFiles()
	if err != nil {
		return fmt.Errorf("failed to list conflicted files: %w", err)
	}

	var unresolved []string
	for _, filePath := range conflictedFiles {
		baseBytes, localBytes, remoteBytes, err := s.git.GetConflictVersions(filePath)
		if err != nil {
			unresolved = append(unresolved, filePath)
			continue
		}

		merged, result, mergeErr := s.merger.Resolve(filePath, baseBytes, localBytes, remoteBytes)
		if mergeErr != nil {
			unresolved = append(unresolved, filePath)
			continue
		}

		if result == merge.Merged {
			if err := s.git.StageFile(filePath, merged); err != nil {
				unresolved = append(unresolved, filePath)
				continue
			}
			fmt.Fprintf(os.Stderr, "  ✓ Smart-merged %s\n", filePath)
			continue
		}

		// Unsupported or HasConflict — fall back to remote for this file.
		unresolved = append(unresolved, filePath)
	}

	// For any files the smart merge couldn't handle, take remote.
	if len(unresolved) > 0 {
		fmt.Fprintf(os.Stderr, "  %d file(s) fell back to remote: %v\n", len(unresolved), unresolved)
		if err := s.git.ResolveConflictsRemote(); err != nil {
			return fmt.Errorf("fallback remote resolution failed: %w", err)
		}
	}

	return nil
}

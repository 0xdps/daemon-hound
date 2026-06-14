package sync

import (
	"fmt"
	"os"
	"time"

	"github.com/0xdps/daemon-hound/internal/git"
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
}

// ConfigReader provides read/write access to local config needed by the syncer.
type ConfigReader interface {
	MachineID() string
	GetBinding(namespace string) (string, bool)
	SetBinding(namespace, localPath string) error
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
	}
}

// WithNamespaceFilter returns a Syncer that only processes the given namespace.
func (s *Syncer) WithNamespaceFilter(ns string) *Syncer {
	copy := *s
	copy.namespaceFilter = ns
	return &copy
}

// shouldProcess reports whether a file should be included given the current filters.
func (s *Syncer) shouldProcess(file models.TrackedFile) bool {
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
	for key, file := range state.Files {
		if !s.shouldProcess(file) {
			continue
		}

		// Only restore if we have a binding for this namespace (or it's global)
		if file.Namespace != "global" {
			if _, ok := s.config.GetBinding(file.Namespace); !ok {
				continue
			}
		}

		localPath, err := s.tracker.Restore(file)
		if err != nil {
			results = append(results, Result{File: file, Action: "error", Error: err})
			continue
		}

		// Update checksum after restore
		checksum, err := utils.FileChecksum(localPath)
		if err != nil {
			results = append(results, Result{File: file, Action: "error", Error: err})
			continue
		}
		file.Checksum = checksum
		file.LastSyncAt = time.Now()
		state.Files[key] = file

		results = append(results, Result{File: file, Action: "pulled"})
	}

	if err := s.vault.SaveState(state); err != nil {
		return results, fmt.Errorf("failed to save vault state: %w", err)
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

		if err := s.vault.StoreFile(file, plaintext); err != nil {
			results = append(results, Result{File: file, Action: "error", Error: err})
			continue
		}

		checksum, err := utils.FileChecksum(localPath)
		if err != nil {
			results = append(results, Result{File: file, Action: "error", Error: err})
			continue
		}
		file.Checksum = checksum
		file.LastSyncAt = time.Now()
		state.Files[key] = file

		results = append(results, Result{File: file, Action: "pushed"})
	}

	if err := s.vault.SaveState(state); err != nil {
		return results, fmt.Errorf("failed to save vault state: %w", err)
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
			fmt.Fprintf(os.Stderr, "Changes committed locally. Run `dh sync` again when online.\n")
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

// Sync runs Pull then Push. If Pull fails (e.g. offline), a warning is printed
// and Push continues so local dirty files are at least committed locally.
func (s *Syncer) Sync() ([]Result, error) {
	pullResults, pullErr := s.Pull()
	if pullErr != nil {
		// Offline or unreachable — warn but continue so local changes are committed.
		fmt.Fprintf(os.Stderr, "Warning: could not pull from remote (offline?): %v\n", pullErr)
		fmt.Fprintf(os.Stderr, "Continuing with local push...\n")
	}
	pushResults, pushErr := s.Push()
	all := append(pullResults, pushResults...)
	if pushErr != nil {
		return all, pushErr
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
	root, ok := s.config.GetBinding(file.Namespace)
	if !ok {
		return "", fmt.Errorf("no local binding for namespace %s", file.Namespace)
	}
	return fmt.Sprintf("%s/%s", root, file.RelPath), nil
}

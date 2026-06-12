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
	vault   *storage.Vault
	tracker *tracker.Tracker
	git     *git.Client
	config  ConfigReader
}

// ConfigReader provides read access to local config.
type ConfigReader interface {
	MachineID() string
	GetBinding(namespace string) (string, bool)
	SetBinding(namespace, localPath string) error
}

// Result holds the outcome of a sync operation for a single file.
type Result struct {
	File   models.TrackedFile
	Action string // "pushed", "pulled", "skipped", "conflict"
	Error  error
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
		// Skip backup files from other machines
		if file.Mode == models.ModeBackup && file.MachineID != s.config.MachineID() {
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
		// Skip backup files from other machines
		if file.Mode == models.ModeBackup && file.MachineID != s.config.MachineID() {
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
			return results, fmt.Errorf("failed to push: %w", err)
		}
	}

	return results, nil
}

// Sync runs Pull then Push.
func (s *Syncer) Sync() ([]Result, error) {
	pullResults, err := s.Pull()
	if err != nil {
		return pullResults, err
	}
	pushResults, err := s.Push()
	if err != nil {
		return append(pullResults, pushResults...), err
	}
	return append(pullResults, pushResults...), nil
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

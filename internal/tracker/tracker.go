// Copyright (C) 2026 DaemonHound Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package tracker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/0xdps/daemon-hound/internal/storage"
	"github.com/0xdps/daemon-hound/internal/utils"
)

// Tracker manages file tracking and dirty-state detection.
type Tracker struct {
	vault  *storage.Vault
	config ConfigReader
}

// ConfigReader provides read access to local config.
type ConfigReader interface {
	MachineID() string
	GetBinding(namespace string) (string, bool)
	SetBinding(namespace, localPath string) error
	UnignoreFile(namespace, relPath string) error
}

// NewTracker creates a new Tracker.
func NewTracker(vault *storage.Vault, config ConfigReader) *Tracker {
	return &Tracker{vault: vault, config: config}
}

// Track adds a file to tracking. It encrypts the file and stores it in the vault.
func (t *Tracker) Track(localPath string, mode models.FileMode) (*models.TrackedFile, error) {
	localPath, err := utils.NormalizePath(localPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("cannot track a directory: %s", localPath)
	}

	// Determine namespace and relative path
	var namespace, relPath string
	if utils.IsInsideGitRepo(filepath.Dir(localPath)) {
		repoRoot, err := utils.FindGitRoot(filepath.Dir(localPath))
		if err != nil {
			return nil, err
		}
		origin, err := utils.GetGitOrigin(repoRoot)
		if err != nil {
			return nil, fmt.Errorf("failed to get git origin: %w", err)
		}
		namespace, err = utils.DeriveNamespace(origin)
		if err != nil {
			return nil, fmt.Errorf("failed to derive namespace: %w", err)
		}
		relPath, err = utils.RelPath(repoRoot, localPath)
		if err != nil {
			return nil, err
		}
		// Record binding
		if err := t.config.SetBinding(namespace, repoRoot); err != nil {
			return nil, fmt.Errorf("failed to save binding: %w", err)
		}
	} else {
		// Global file — store path relative to $HOME to preserve subdirectory structure.
		namespace = "global"
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", homeErr)
		}
		rel, relErr := filepath.Rel(home, localPath)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			// File is outside $HOME — fall back to base name.
			rel = filepath.Base(localPath)
		}
		relPath = rel
	}

	// Read and encrypt file
	plaintext, err := os.ReadFile(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	checksum, err := utils.FileChecksum(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to compute checksum: %w", err)
	}

	file := models.TrackedFile{
		Namespace: namespace,
		RelPath:   relPath,
		Mode:      mode,
		Checksum:  checksum,
	}
	// Set MachineID for backup mode OR global files (global files are always machine-specific)
	if mode == models.ModeBackup || namespace == "global" {
		file.MachineID = t.config.MachineID()
	}

	if _, err := t.vault.StoreFile(file, plaintext); err != nil {
		return nil, fmt.Errorf("failed to store file in vault: %w", err)
	}

	if err := t.config.UnignoreFile(file.Namespace, file.RelPath); err != nil {
		return nil, fmt.Errorf("failed to clear local ignore: %w", err)
	}

	return &file, nil
}

// Status returns the dirty status of a tracked file on disk.
func (t *Tracker) Status(file models.TrackedFile) (models.DirtyStatus, error) {
	localPath, err := t.resolveLocalPath(file)
	if err != nil {
		return models.StatusMissing, nil // binding missing means file not present
	}

	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		return models.StatusMissing, nil
	}

	// No checksum stored yet — file tracked but never synced
	if file.Checksum == "" {
		return models.StatusNew, nil
	}

	checksum, err := utils.FileChecksum(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to compute checksum: %w", err)
	}

	if checksum != file.Checksum {
		return models.StatusDirty, nil
	}
	return models.StatusClean, nil
}

// Restore decrypts a tracked file from the vault and writes it to the local path.
func (t *Tracker) Restore(file models.TrackedFile) (string, error) {
	localPath, err := t.resolveLocalPath(file)
	if err != nil {
		return "", err
	}

	plaintext, err := t.vault.RetrieveFile(file)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve file from vault: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create local directory: %w", err)
	}
	if err := os.WriteFile(localPath, plaintext, 0600); err != nil {
		return "", fmt.Errorf("failed to write local file: %w", err)
	}

	return localPath, nil
}

// resolveLocalPath determines the local filesystem path for a tracked file.
func (t *Tracker) resolveLocalPath(file models.TrackedFile) (string, error) {
	if file.Namespace == "global" {
		// For global files, use home directory as base
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, file.RelPath), nil
	}

	if root, ok := t.config.GetBinding(file.Namespace); ok {
		return filepath.Join(root, file.RelPath), nil
	}

	root, err := t.discoverBinding(file.Namespace)
	if err != nil {
		return "", fmt.Errorf("no local binding for namespace %s: %w", file.Namespace, err)
	}
	_ = t.config.SetBinding(file.Namespace, root)
	return filepath.Join(root, file.RelPath), nil
}

func (t *Tracker) discoverBinding(namespace string) (string, error) {
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

// ResolveKey determines the vault state key (namespace:relPath) for a local file path
// without performing any writes or encryption.
func (t *Tracker) ResolveKey(localPath string) (namespace, relPath string, err error) {
	localPath, err = utils.NormalizePath(localPath)
	if err != nil {
		return
	}
	if utils.IsInsideGitRepo(filepath.Dir(localPath)) {
		var repoRoot, origin string
		repoRoot, err = utils.FindGitRoot(filepath.Dir(localPath))
		if err != nil {
			return
		}
		origin, err = utils.GetGitOrigin(repoRoot)
		if err != nil {
			return
		}
		namespace, err = utils.DeriveNamespace(origin)
		if err != nil {
			return
		}
		relPath, err = utils.RelPath(repoRoot, localPath)
		return
	}
	namespace = "global"
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		err = fmt.Errorf("failed to get home directory: %w", homeErr)
		return
	}
	rel, relErr := filepath.Rel(home, localPath)
	if relErr != nil || strings.HasPrefix(rel, "..") {
		rel = filepath.Base(localPath)
	}
	relPath = rel
	return
}

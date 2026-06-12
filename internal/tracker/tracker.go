package tracker

import (
	"fmt"
	"os"
	"path/filepath"

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
		// Global file
		namespace = "global"
		relPath = filepath.Base(localPath)
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
	if mode == models.ModeBackup {
		file.MachineID = t.config.MachineID()
	}

	if err := t.vault.StoreFile(file, plaintext); err != nil {
		return nil, fmt.Errorf("failed to store file in vault: %w", err)
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

	root, ok := t.config.GetBinding(file.Namespace)
	if !ok {
		return "", fmt.Errorf("no local binding for namespace %s", file.Namespace)
	}
	return filepath.Join(root, file.RelPath), nil
}

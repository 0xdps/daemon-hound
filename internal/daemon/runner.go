package daemon

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/conflicts"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/merge"
	"github.com/0xdps/daemon-hound/internal/storage"
	dhsync "github.com/0xdps/daemon-hound/internal/sync"
	"github.com/0xdps/daemon-hound/internal/tracker"
)

// Runner manages the daemon background syncing process.
type Runner struct {
	cfg        *config.Config
	vault      *storage.Vault // nil if identity could not be loaded
	syncer     *dhsync.Syncer // nil if identity could not be loaded
	logger     *log.Logger
	errFile    *os.File
	stopCh     chan struct{}
	logRotator *LogRotator
	merger     *merge.Registry
	halted     bool // true when suspended waiting for conflict resolution
}

// NewRunner creates a new daemon runner.
// vault and syncer may be nil when the identity cannot be loaded; dirty-file encryption
// and smart merge are skipped in that case.
func NewRunner(cfg *config.Config, vault *storage.Vault, syncer *dhsync.Syncer) (*Runner, error) {
	logPath, err := getLogPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get log path: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	errPath, err := getErrorLogPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get error log path: %w", err)
	}

	errFile, err := os.OpenFile(errPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open error log file: %w", err)
	}

	logger := log.New(logFile, "[daemon] ", log.LstdFlags)

	merger := merge.NewRegistry()
	if vault != nil {
		merger = merger.WithSecretDriver(vault.Encrypt, vault.Decrypt)
	}

	return &Runner{
		cfg:        cfg,
		vault:      vault,
		syncer:     syncer,
		logger:     logger,
		errFile:    errFile,
		stopCh:     make(chan struct{}),
		logRotator: NewLogRotator(10, 5, 30), // 10MB, 5MB, 30 days default
		merger:     merger,
	}, nil
}

// Run starts the daemon syncing loop. Blocks until stopped via context or signal.
func (r *Runner) Run(ctx context.Context) error {
	defer r.errFile.Close()

	r.logger.Println("=== Daemon started ===")

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create polling ticker
	pollTicker := time.NewTicker(30 * time.Second)
	defer pollTicker.Stop()

	// Create log rotation ticker (check hourly)
	logRotateTicker := time.NewTicker(1 * time.Hour)
	defer logRotateTicker.Stop()

	// Restore halt state from a previous run — if the sentinel file exists
	// the user hasn't resolved conflicts yet; start in halted mode.
	if IsHalted() {
		reason := ReadHaltReason()
		r.halted = true
		r.logger.Printf("Daemon starting in halted state: %s", reason)
		r.logger.Println("Resolve conflicts with: dhd conflicts list")
	}

	// Initial sync
	r.logger.Println("Performing initial sync...")
	if err := r.performSync(); err != nil {
		r.logger.Printf("Initial sync failed: %v", err)
	}

	// Main loop
	for {
		select {
		case <-ctx.Done():
			r.logger.Println("=== Daemon stopped (context) ===")
			return ctx.Err()

		case <-r.stopCh:
			r.logger.Println("=== Daemon stopped (signal) ===")
			return nil

		case sig := <-sigCh:
			r.logger.Printf("Received signal: %v", sig)
			r.logger.Println("=== Daemon stopped ===")
			return nil

		case <-pollTicker.C:
			// If the daemon is halted due to unresolved conflicts, skip sync
			// entirely — re-running will just hit the same conflict and waste
			// compute and network. Wait for user to run `dhd conflicts resolve`.
			if r.halted {
				r.logger.Println("Sync skipped: daemon halted (unresolved conflicts — run: dhd conflicts list)")
				continue
			}
			r.logger.Println("Running scheduled poll...")
			if err := r.performSync(); err != nil {
				r.logger.Printf("Scheduled poll failed: %v", err)
			}

		case <-logRotateTicker.C:
			if err := r.logRotator.Rotate(); err != nil {
				r.logger.Printf("Log rotation failed: %v", err)
			}
		}
	}
}

// Stop gracefully stops the daemon.
func (r *Runner) Stop() {
	close(r.stopCh)
}

// performSync orchestrates the full sync operation.
func (r *Runner) performSync() error {
	startTime := time.Now()
	vaultRemote := r.cfg.VaultRemote()
	r.logger.Printf("Starting sync (vault: %s)", vaultRemote)

	// Reload config from disk on every sync cycle so that bindings added by
	// `dhd track` or `dhd discover` after the daemon started are picked up
	// immediately rather than requiring a daemon restart.
	freshCfg := config.NewConfig()
	if err := freshCfg.Load(); err != nil {
		r.logger.Printf("Warning: failed to reload config: %v", err)
		freshCfg = r.cfg // fall back to startup config
	}

	// Rebuild syncer with fresh config if vault identity is available.
	var syncer *dhsync.Syncer
	if r.vault != nil {
		vaultPath := config.VaultPath()
		gcSyncer := git.NewClient(vaultPath)
		tr := tracker.NewTracker(r.vault, freshCfg)
		syncer = dhsync.NewSyncer(r.vault, tr, gcSyncer, freshCfg).WithMerger(r.merger)
	}

	// Create git client for vault repository
	vaultPath := config.VaultPath()
	gc := git.NewClient(vaultPath)

	// Pre-flight: recover from a stuck in-progress merge before doing anything.
	// Try smart merge first; fall back to remote for anything the drivers can't handle.
	if gc.IsInMerge() {
		if hasConflicts, _ := gc.HasConflicts(); hasConflicts {
			r.logger.Println("Recovering from stuck merge — attempting smart merge")
			if err := r.resolveConflicts(gc, vaultPath); err != nil {
				r.logger.Printf("Warning: smart merge failed: %v — falling back to remote", err)
				if err := gc.ResolveConflictsRemote(); err != nil {
					r.logger.Printf("Warning: conflict resolution failed: %v — aborting merge", err)
					_ = gc.AbortMerge()
				} else {
					_ = gc.CommitAll("[daemon] Resolve vault merge conflicts")
				}
			} else {
				_ = gc.CommitAll("[daemon] Resolve vault merge conflicts [smart]")
			}
		} else {
			_ = gc.CommitAll("[daemon] Complete in-progress merge")
		}
	}

	// Pull changes from remote, restore files to disk, then push any local
	// dirty files. Using syncer.Sync() ensures tracker.Restore() is called
	// so that remote changes (e.g. from another machine) are actually written
	// to the tracked file paths — a raw gc.Pull() only downloads the .age
	// files but never restores them, causing a continuous dirty→push loop.
	if syncer != nil {
		results, err := syncer.Sync()
		if err != nil {
			r.logger.Printf("Warning: sync failed: %v", err)
		}
		for _, res := range results {
			if res.Error != nil {
				r.logger.Printf("Error syncing %s/%s: %v", res.File.Namespace, res.File.RelPath, res.Error)
			} else if res.Action == "pushed" {
				r.logger.Printf("Pushed: %s/%s", res.File.Namespace, res.File.RelPath)
			} else if res.Action == "pulled" {
				r.logger.Printf("Pulled: %s/%s", res.File.Namespace, res.File.RelPath)
			}
		}
	} else {
		// No vault identity — do a raw pull and commit any pre-existing changes.
		if err := gc.Pull(); err != nil {
			r.logger.Printf("Warning: failed to pull from remote: %v", err)
		} else {
			r.logger.Println("Pulled from remote")
		}
		if hasChanges, err := gc.HasChanges(); err == nil && hasChanges {
			if err := gc.CommitAll("[daemon] Sync local changes"); err != nil {
				r.logger.Printf("Failed to commit changes: %v", err)
			} else if err := gc.Push(); err != nil {
				r.logger.Printf("Failed to push to remote: %v", err)
			}
		}
	}

	elapsed := time.Since(startTime)
	r.logger.Printf("Sync completed in %v", elapsed)
	return nil
}

// resolveConflicts attempts smart merge for each conflicted file.
// Files that merge cleanly are staged automatically.
// Files with true conflicts are recorded in the conflicts store for user review.
func (r *Runner) resolveConflicts(gc *git.Client, vaultPath string) error {
	conflictedFiles, err := gc.GetConflictedFiles()
	if err != nil {
		return fmt.Errorf("failed to list conflicted files: %w", err)
	}

	store, storeErr := conflicts.NewStore()

	for _, filePath := range conflictedFiles {
		fullPath := filepath.Join(vaultPath, filePath)
		localHash := hashFile(fullPath)
		baseBytes, localBytes, remoteBytes, err := gc.GetConflictVersions(filePath)
		if err != nil {
			r.logger.Printf("[conflict] Failed to get git versions of %s: %v", filePath, err)
			r.recordConflict(store, storeErr, filePath, localHash, "")
			continue
		}

		merged, result, mergeErr := r.merger.Resolve(filePath, baseBytes, localBytes, remoteBytes)
		if mergeErr != nil {
			r.logger.Printf("[conflict] Merge error for %s: %v", filePath, mergeErr)
			r.recordConflict(store, storeErr, filePath, localHash, "")
			continue
		}

		if result == merge.Merged {
			if err := gc.StageFile(filePath, merged); err != nil {
				r.logger.Printf("[conflict] Stage failed for %s: %v", filePath, err)
				r.recordConflict(store, storeErr, filePath, localHash, "")
				continue
			}
			r.logger.Printf("[conflict] Smart-merged %s successfully", filePath)
			if store != nil {
				_ = store.Delete(filePath)
			}
			continue
		}

		// Smart merge not possible — record for user
		remoteHash := hashBytes(remoteBytes)
		r.logger.Printf("[conflict] True conflict in %s — user action required: dhd conflicts show %s", filePath, filePath)
		r.recordConflict(store, storeErr, filePath, localHash, remoteHash)
	}

	return nil
}

// recordConflict saves conflict metadata to the store for user review.
// If any true conflict is recorded, the daemon halts sync until the user resolves it.
func (r *Runner) recordConflict(store *conflicts.Store, storeErr error, filePath, localHash, remoteHash string) {
	if storeErr != nil || store == nil {
		return
	}
	c := &conflicts.Conflict{
		FilePath:   filePath,
		DetectedAt: time.Now(),
		LocalHash:  localHash,
		RemoteHash: remoteHash,
	}
	if err := store.Add(c); err != nil {
		r.logger.Printf("Warning: failed to record conflict %s: %v", filePath, err)
		return
	}
	// Halt the daemon so it stops burning CPU/network on a conflict it can't fix.
	if !r.halted {
		reason := fmt.Sprintf("unresolved conflict in %s — run: dhd conflicts list", filePath)
		if err := WriteHalt(reason); err != nil {
			r.logger.Printf("Warning: could not write halt sentinel: %v", err)
		}
		r.halted = true
		r.logger.Printf("⚠ Daemon halted: %s", reason)
		r.logger.Println("  Resolve with: dhd conflicts list && dhd conflicts resolve <file> --strategy local|remote")
		r.logger.Println("  Then resume with: dhd daemon resume")
	}
}

// Helper to get log path
func getLogPath() (string, error) {
	dhPath := filepath.Join(os.Getenv("HOME"), ".dh")
	return filepath.Join(dhPath, "daemon.log"), nil
}

// Helper to get error log path
func getErrorLogPath() (string, error) {
	dhPath := filepath.Join(os.Getenv("HOME"), ".dh")
	return filepath.Join(dhPath, "daemon.error.log"), nil
}

// hashFile hashes the current on-disk content of a file for conflict tracking.
func hashFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "unreadable"
	}
	return hashBytes(data)
}

func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:])[:16]
}

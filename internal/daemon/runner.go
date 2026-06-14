package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/fsnotify/fsnotify"
)

// Runner manages the daemon background syncing process.
type Runner struct {
	cfg        *config.Config
	logger     *log.Logger
	errFile    *os.File
	watcher    *fsnotify.Watcher
	stopCh     chan struct{}
	syncOnce   sync.Mutex
	logRotator *LogRotator
}

// NewRunner creates a new daemon runner.
func NewRunner(cfg *config.Config) (*Runner, error) {
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

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	logger := log.New(logFile, "[daemon] ", log.LstdFlags)

	return &Runner{
		cfg:        cfg,
		logger:     logger,
		errFile:    errFile,
		watcher:    watcher,
		stopCh:     make(chan struct{}),
		logRotator: NewLogRotator(10, 5, 30), // 10MB, 5MB, 30 days default
	}, nil
}

// Run starts the daemon syncing loop. Blocks until stopped via context or signal.
func (r *Runner) Run(ctx context.Context) error {
	defer func() {
		r.watcher.Close()
		r.errFile.Close()
	}()

	r.logger.Println("=== Daemon started ===")

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Watch vault directory for changes
	vaultPath := filepath.Join(os.Getenv("HOME"), ".dh", "vault")
	if err := filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return r.watcher.Add(path)
		}
		return nil
	}); err != nil {
		r.logger.Printf("Warning: failed to watch vault directory: %v", err)
	}

	// Create polling ticker
	pollTicker := time.NewTicker(30 * time.Second)
	defer pollTicker.Stop()

	// Create log rotation ticker (check hourly)
	logRotateTicker := time.NewTicker(1 * time.Hour)
	defer logRotateTicker.Stop()

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

		case event, ok := <-r.watcher.Events:
			if !ok {
				return fmt.Errorf("watcher events channel closed")
			}
			if r.isRelevantChange(event.Name) {
				r.logger.Printf("Detected local change: %s (%s)", event.Name, event.Op)
				r.triggerSync()
			}

		case err, ok := <-r.watcher.Errors:
			if !ok {
				return fmt.Errorf("watcher errors channel closed")
			}
			fmt.Fprintf(r.errFile, "[watcher] %v\n", err)

		case <-pollTicker.C:
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

// isRelevantChange checks if a file change is relevant for syncing.
func (r *Runner) isRelevantChange(path string) bool {
	// Ignore dotfiles and temporary files
	base := filepath.Base(path)
	if len(base) > 0 && base[0] == '.' {
		return false
	}
	if len(base) > 0 && base[len(base)-1] == '~' {
		return false
	}

	// Ignore .git directory
	if containsPath(path, ".git") {
		return false
	}

	return true
}

// triggerSync performs a sync with debouncing (waits 2 seconds for more changes).
func (r *Runner) triggerSync() {
	r.syncOnce.Lock()
	defer r.syncOnce.Unlock()

	// Debounce: wait a bit for more changes to accumulate
	time.Sleep(2 * time.Second)

	if err := r.performSync(); err != nil {
		r.logger.Printf("Triggered sync failed: %v", err)
	}
}

// performSync orchestrates the full sync operation.
func (r *Runner) performSync() error {
	startTime := time.Now()
	vaultRemote := r.cfg.VaultRemote()
	r.logger.Printf("Starting sync (vault: %s)", vaultRemote)

	// Create git client for vault repository
	vaultPath := filepath.Join(os.Getenv("HOME"), ".dh", "vault")
	gc := git.NewClient(vaultPath)

	// Pull changes from remote (includes fetch)
	if err := gc.Pull(); err != nil {
		r.logger.Printf("Warning: failed to pull from remote: %v", err)
	} else {
		r.logger.Println("Pulled from remote")
	}

	// Check for merge conflicts
	hasConflicts, err := gc.HasConflicts()
	if err != nil {
		r.logger.Printf("Warning: failed to check for conflicts: %v", err)
	} else if hasConflicts {
		r.logger.Println("Merge conflicts detected, resolving...")

		conflicted, err := gc.GetConflictedFiles()
		if err != nil {
			r.logger.Printf("Warning: failed to get conflicted files: %v", err)
		} else {
			r.logger.Printf("Conflicted files: %v", conflicted)
		}

		// Use "local" strategy by default (keep local changes)
		strategy := "local"
		if err := gc.ResolveConflicts(strategy); err != nil {
			r.logger.Printf("Failed to resolve conflicts: %v", err)
			return nil
		}

		r.logger.Printf("Resolved conflicts using '%s' strategy", strategy)

		// Commit the resolution
		if err := gc.CommitAll("[daemon] Resolve merge conflicts"); err != nil {
			r.logger.Printf("Failed to commit conflict resolution: %v", err)
		} else {
			r.logger.Println("Committed conflict resolution")
		}
	}

	// Check for local changes
	hasChanges, err := gc.HasChanges()
	if err != nil {
		r.logger.Printf("Warning: failed to check for local changes: %v", err)
	} else if hasChanges {
		r.logger.Println("Detected local changes, committing...")

		// Commit local changes
		if err := gc.CommitAll("[daemon] Sync local changes"); err != nil {
			r.logger.Printf("Failed to commit changes: %v", err)
		} else {
			r.logger.Println("Committed local changes")

			// Push to remote
			if err := gc.Push(); err != nil {
				r.logger.Printf("Failed to push to remote: %v", err)
			} else {
				r.logger.Println("Pushed to remote")
			}
		}
	}

	elapsed := time.Since(startTime)
	r.logger.Printf("Sync completed in %v", elapsed)
	return nil
}

// containsPath checks if a path contains a given component
func containsPath(path, component string) bool {
	parts := filepath.SplitList(path)
	for _, part := range parts {
		if part == component {
			return true
		}
	}
	return false
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

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

package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/conflicts"
	"github.com/0xdps/daemon-hound/internal/daemon"
	"github.com/0xdps/daemon-hound/internal/git"
	dhsync "github.com/0xdps/daemon-hound/internal/sync"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the background sync daemon",
	Long:  `Manage the background sync daemon for automatic vault synchronization.`,
}

var daemonRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the daemon in foreground (for testing)",
	Long:  `Start the daemon in foreground mode for testing. In production, the daemon is managed by the OS service manager (launchd, systemd, etc.)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.NewConfig()
		if err := cfg.Load(); err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Load vault identity for smart conflict resolution and dirty-file encryption (best-effort).
		// If the identity cannot be loaded the daemon still runs, but dirty-file encryption
		// and smart merge are disabled until the next restart.
		cfg2, vault, tr, err := loadContext()
		var syncer *dhsync.Syncer
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load vault identity (%v) — file sync and smart merge disabled\n", err)
			vault = nil
		} else {
			gc := git.NewClient(config.VaultPath())
			syncer = dhsync.NewSyncer(vault, tr, gc, cfg2)
		}

		runner, err := daemon.NewRunner(cfg, vault, syncer)
		if err != nil {
			return fmt.Errorf("failed to create daemon runner: %w", err)
		}

		// Run with context that can be cancelled
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		return runner.Run(ctx)
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status (running / stopped / halted)",
	Long:  `Show the current state of the background sync daemon. Does NOT start the daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sm := daemon.NewServiceManager()

		installed, err := sm.IsInstalled()
		if err != nil {
			return fmt.Errorf("failed to check service installation: %w", err)
		}
		if !installed {
			fmt.Println("❌ Daemon not installed")
			fmt.Println("   Run: dhd init --remote <url>")
			return nil
		}

		fmt.Println("✓ Daemon service installed")

		// Halted state takes priority — report it even if the process is "running"
		if daemon.IsHalted() {
			reason := daemon.ReadHaltReason()
			fmt.Println("⛔ Daemon halted (sync suspended)")
			if reason != "" {
				fmt.Printf("   Reason: %s\n", reason)
			}
			fmt.Println("   Resolve conflicts: dhd conflicts list")
			fmt.Println("   Then resume:       dhd daemon resume")
			return nil
		}

		running, err := sm.IsRunning()
		if err != nil {
			fmt.Printf("⚠️  Could not determine if daemon is running: %v\n", err)
			return nil
		}
		if running {
			fmt.Println("✓ Daemon is running")
			fmt.Println("  Syncing every 30 seconds")
		} else {
			fmt.Println("⚠️  Daemon is stopped")
			fmt.Println("   Start it: dhd daemon start")
		}
		return nil
	},
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the background daemon",
	Long:  `Install and start the background sync daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if daemon.IsHalted() {
			reason := daemon.ReadHaltReason()
			fmt.Println("⛔ Cannot start: daemon is halted due to unresolved conflicts")
			if reason != "" {
				fmt.Printf("   Reason: %s\n", reason)
			}
			fmt.Println("   Resolve: dhd conflicts list")
			fmt.Println("   Resume:  dhd daemon resume")
			return nil
		}
		sm := daemon.NewServiceManager()
		running, _ := sm.IsRunning()
		if running {
			fmt.Println("Daemon is already running")
			return nil
		}
		if err := sm.Install(); err != nil {
			return fmt.Errorf("failed to start daemon: %w", err)
		}
		fmt.Println("✓ Daemon started")
		return nil
	},
}

var daemonResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume sync after resolving conflicts",
	Long: `Clear the halt state and resume automatic syncing.

Only run this after resolving all pending conflicts:
  dhd conflicts list
  dhd conflicts resolve <file> --strategy local|remote`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !daemon.IsHalted() {
			fmt.Println("Daemon is not halted — nothing to resume")
			return nil
		}

		// Check there are no pending conflicts before allowing resume
		store, err := conflicts.NewStore()
		if err == nil {
			pending, _ := store.Pending()
			if len(pending) > 0 {
				fmt.Printf("⚠️  %d conflict(s) still pending resolution:\n", len(pending))
				for _, c := range pending {
					fmt.Printf("   - %s\n", c.FilePath)
				}
				fmt.Println("\nResolve them first: dhd conflicts resolve <file> --strategy local|remote")
				return nil
			}
		}

		if err := daemon.ClearHalt(); err != nil {
			return fmt.Errorf("failed to clear halt: %w", err)
		}
		fmt.Println("✓ Halt cleared — daemon will resume syncing on next poll")
		fmt.Println("  (Or restart it now: dhd daemon restart)")
		return nil
	},
}

var daemonLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View daemon logs",
	Long:  `Display daemon activity logs. Use -f to follow in real-time.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		follow, _ := cmd.Flags().GetBool("follow")
		lines, _ := cmd.Flags().GetInt("lines")

		logPath := filepath.Join(os.Getenv("HOME"), ".daemon-hound", "daemon.log")

		// Check if log file exists
		if _, err := os.Stat(logPath); err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("No logs yet. Daemon hasn't run.\n")
				fmt.Println("Start daemon with: dhd daemon run")
				return nil
			}
			return fmt.Errorf("failed to check log file: %w", err)
		}

		if follow {
			return tailLogs(logPath, lines)
		}

		// Show last N lines
		return showLogTail(logPath, lines)
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background daemon",
	Long:  `Stop the background sync daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sm := daemon.NewServiceManager()

		running, err := sm.IsRunning()
		if err != nil {
			return fmt.Errorf("failed to check daemon status: %w", err)
		}

		if !running {
			fmt.Println("Daemon is not running")
			return nil
		}

		// Uninstall stops the service
		if err := sm.Uninstall(); err != nil {
			return fmt.Errorf("failed to stop daemon: %w", err)
		}

		fmt.Println("✓ Daemon stopped")
		return nil
	},
}

var daemonRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the background daemon",
	Long:  `Stop and restart the background sync daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sm := daemon.NewServiceManager()

		// Stop
		if running, _ := sm.IsRunning(); running {
			if err := sm.Uninstall(); err != nil {
				return fmt.Errorf("failed to stop daemon: %w", err)
			}
			fmt.Println("✓ Daemon stopped")
			time.Sleep(1 * time.Second) // Give it a moment
		}

		// Start
		if err := sm.Install(); err != nil {
			return fmt.Errorf("failed to start daemon: %w", err)
		}

		fmt.Println("✓ Daemon restarted")
		return nil
	},
}

// tailLogs follows log file (like tail -f)
func tailLogs(logPath string, lines int) error {
	fmt.Printf("Following %s (Ctrl+C to exit)...\n\n", logPath)

	// Show initial lines
	if err := showLogTail(logPath, lines); err != nil {
		return err
	}

	// Watch for new lines
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastSize := int64(0)
	if info, err := os.Stat(logPath); err == nil {
		lastSize = info.Size()
	}

	for range ticker.C {
		info, err := os.Stat(logPath)
		if err != nil {
			return err
		}

		if info.Size() > lastSize {
			// File grew, show new content
			if err := showNewLines(logPath, lastSize); err != nil {
				return err
			}
			lastSize = info.Size()
		}
	}

	return nil
}

// showLogTail shows last N lines of log file
func showLogTail(logPath string, lines int) error {
	content, err := os.ReadFile(logPath)
	if err != nil {
		return err
	}

	logLines := strings.Split(strings.TrimSpace(string(content)), "\n")
	start := len(logLines) - lines
	if start < 0 {
		start = 0
	}

	for i := start; i < len(logLines); i++ {
		fmt.Println(logLines[i])
	}

	return nil
}

// showNewLines shows new lines appended since lastSize
func showNewLines(logPath string, lastSize int64) error {
	file, err := os.Open(logPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Seek(lastSize, 0); err != nil {
		return err
	}

	content := make([]byte, 4096)
	n, err := file.Read(content)
	if err != nil && err.Error() != "EOF" {
		return err
	}

	if n > 0 {
		fmt.Print(string(content[:n]))
	}

	return nil
}

// daemonErrorLogsCmd shows error logs
var daemonErrorLogsCmd = &cobra.Command{
	Use:   "errors",
	Short: "View daemon error logs",
	Long:  `Display daemon error logs.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		errLogPath := filepath.Join(os.Getenv("HOME"), ".daemon-hound", "daemon.error.log")

		// Check if log file exists
		if _, err := os.Stat(errLogPath); err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No errors logged yet.")
				return nil
			}
			return fmt.Errorf("failed to check error log file: %w", err)
		}

		return showLogTail(errLogPath, 100)
	},
}

func init() {
	daemonCmd.AddCommand(daemonRunCmd)
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonLogsCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonRestartCmd)
	daemonCmd.AddCommand(daemonResumeCmd)
	daemonCmd.AddCommand(daemonErrorLogsCmd)

	// Flags for logs command
	daemonLogsCmd.Flags().BoolP("follow", "f", false, "Follow log file in real-time")
	daemonLogsCmd.Flags().IntP("lines", "n", 50, "Number of lines to show")

	rootCmd.AddCommand(daemonCmd)
}

package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xdps/daemon-hound/internal/config"
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
	Short: "Check if the daemon is running",
	Long:  `Check the status of the background sync daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sm := daemon.NewServiceManager()

		// Check if service is installed
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

		// Check if running
		running, err := sm.IsRunning()
		if err != nil {
			fmt.Printf("⚠️  Could not determine if daemon is running: %v\n", err)
			return nil
		}

		if running {
			fmt.Println("✓ Daemon is running")
			fmt.Println("  Syncing every 30 seconds")
		} else {
			fmt.Println("⚠️  Daemon is not running")
			fmt.Println("  Trying to start daemon...")

			// Try to start it
			if err := sm.Install(); err != nil {
				fmt.Printf("❌ Failed to start daemon: %v\n", err)
				fmt.Println("   Run: dhd daemon run (for manual testing)")
				return nil
			}

			fmt.Println("✓ Daemon started")
		}

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

		logPath := filepath.Join(os.Getenv("HOME"), ".dh", "daemon.log")

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
		errLogPath := filepath.Join(os.Getenv("HOME"), ".dh", "daemon.error.log")

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
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonLogsCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonRestartCmd)
	daemonCmd.AddCommand(daemonErrorLogsCmd)

	// Flags for logs command
	daemonLogsCmd.Flags().BoolP("follow", "f", false, "Follow log file in real-time")
	daemonLogsCmd.Flags().IntP("lines", "n", 50, "Number of lines to show")

	rootCmd.AddCommand(daemonCmd)
}

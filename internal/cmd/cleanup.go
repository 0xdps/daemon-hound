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
	"fmt"
	"os"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/daemon"
	"github.com/0xdps/daemon-hound/internal/keychain"
	"github.com/0xdps/daemon-hound/internal/output"
	"github.com/0xdps/daemon-hound/internal/utils"
	"github.com/spf13/cobra"
)

var cleanupForce bool

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Completely remove DaemonHound from this machine",
	Long: `Completely remove DaemonHound from this machine.

This command:
  1. Stops and uninstalls the background sync daemon
  2. Removes the master password from the OS keychain (logout)
  3. Removes all local data under ~/.dh
     • vault/        — local vault clone
     • config.toml   — machine configuration
     • identity.age  — encrypted identity key
     • daemon logs   — daemon.log, daemon.error.log
     • sync.lock     — process lock file
     • audit.log     — audit log

This does NOT affect your remote vault repository. Your tracked files
and secrets remain safe in the remote git repository.

To re-initialize on this machine later, run:
  dhd init --remote <your-vault-url>`,
	RunE: runCleanup,
}

func init() {
	cleanupCmd.Flags().BoolVar(&cleanupForce, "force", false, "Skip confirmation prompt")
	rootCmd.AddCommand(cleanupCmd)
}

func runCleanup(cmd *cobra.Command, args []string) error {
	appDir := config.AppDir()

	appDirExists := true
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		appDirExists = false
	}

	if !appDirExists && !keychain.IsSet() {
		fmt.Println("DaemonHound is not initialized on this machine.")
		return nil
	}

	// Prompt for confirmation unless --force
	if !cleanupForce {
		fmt.Printf("This will completely remove DaemonHound from this machine:\n")
		fmt.Printf("  • Stop and uninstall the background sync daemon\n")
		fmt.Printf("  • Remove the master password from keychain\n")
		fmt.Printf("  • Delete all local data under %s\n\n", appDir)
		fmt.Println("Your remote vault repository will NOT be affected.")
		confirm, err := utils.PromptInput("Are you sure? (yes/no): ")
		if err != nil {
			return err
		}
		if confirm != "yes" && confirm != "y" {
			fmt.Println("Cleanup cancelled.")
			return nil
		}
	}

	// Step 1: Stop and uninstall the daemon
	sm := daemon.NewServiceManager()
	if installed, err := sm.IsInstalled(); err == nil && installed {
		if err := sm.Uninstall(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to uninstall daemon: %v\n", err)
		} else {
			fmt.Println(output.Green("✓") + " Daemon stopped and uninstalled")
		}
	} else {
		fmt.Println(output.Dim("  Daemon not installed, skipping"))
	}

	if err := daemon.RemoveAppBundle(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove app bundle: %v\n", err)
	} else {
		fmt.Println(output.Green("✓") + " Removed macOS app bundle")
	}

	// Step 2: Remove master password from keychain (logout)
	if keychain.IsSet() {
		if err := keychain.Delete(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove keychain entry: %v\n", err)
		} else {
			fmt.Println(output.Green("✓") + " Removed master password from keychain")
		}
	} else {
		fmt.Println(output.Dim("  No keychain entry, skipping"))
	}

	// Step 3: Remove all local data
	if appDirExists {
		if err := os.RemoveAll(appDir); err != nil {
			return fmt.Errorf("failed to remove %s: %w", appDir, err)
		}
		fmt.Printf("%s Removed local data (%s)\n", output.Green("✓"), appDir)
	}

	fmt.Println("\n" + output.Bold("DaemonHound has been completely removed from this machine."))
	fmt.Println(output.Dim("Your remote vault repository remains intact."))
	fmt.Printf("\nTo re-initialize, run: %s\n", output.Cyan("dhd init --remote <your-vault-url>"))
	return nil
}

package cmd

import (
	"fmt"
	"os"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/keychain"
	"github.com/0xdps/daemon-hound/internal/output"
	"github.com/0xdps/daemon-hound/internal/utils"
	"github.com/spf13/cobra"
)

var cleanupForce bool

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove all local DaemonHound data (vault, config, identity, keychain)",
	Long: `Completely remove DaemonHound from this machine.

This command removes:
  • ~/.dh/vault (local vault clone)
  • ~/.dh/config.toml (machine configuration)
  • ~/.dh/identity.age (encrypted identity key)
  • ~/.dh/audit.log (audit log)
  • ~/.dh/sync.lock (process lock file)
  • Master password from OS keychain

This does NOT affect your remote vault repository. Your tracked files
and vault state remain safe in the remote git repository.

To re-initialize later, run 'dh init' with the same vault remote.`,
	RunE: runCleanup,
}

func init() {
	cleanupCmd.Flags().BoolVar(&cleanupForce, "force", false, "Skip confirmation prompt")
	rootCmd.AddCommand(cleanupCmd)
}

func runCleanup(cmd *cobra.Command, args []string) error {
	appDir := config.AppDir()

	// Check if already clean
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		if !keychain.IsSet() {
			fmt.Println("DaemonHound is not initialized on this machine.")
			return nil
		}
		// Only keychain entry exists
		fmt.Println("No local data found, but keychain entry exists.")
		if !cleanupForce {
			confirm, err := utils.PromptInput("Remove keychain entry? (yes/no): ")
			if err != nil {
				return err
			}
			if confirm != "yes" && confirm != "y" {
				fmt.Println("Cleanup cancelled.")
				return nil
			}
		}
		if err := keychain.Delete(); err != nil && keychain.IsSet() {
			return fmt.Errorf("failed to remove keychain entry: %w", err)
		}
		fmt.Println("Keychain entry removed.")
		return nil
	}

	// Prompt for confirmation unless --force
	if !cleanupForce {
		fmt.Printf("This will permanently remove all local DaemonHound data from:\n  %s\n\n", appDir)
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

	// Remove keychain entry
	if keychain.IsSet() {
		if err := keychain.Delete(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove keychain entry: %v\n", err)
		} else {
			fmt.Println(output.Green("✓") + " Removed master password from keychain")
		}
	}

	// Remove entire ~/.dh directory
	if err := os.RemoveAll(appDir); err != nil {
		return fmt.Errorf("failed to remove %s: %w", appDir, err)
	}

	fmt.Printf("%s Removed all local data from %s\n", output.Green("✓"), appDir)
	fmt.Println("\n" + output.Bold("DaemonHound has been completely removed from this machine."))
	fmt.Println(output.Dim("Your remote vault repository remains intact."))
	fmt.Printf("\nTo re-initialize, run: %s\n", output.Cyan("dh init --remote <your-vault-url>"))

	return nil
}

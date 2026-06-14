package cmd

import (
	"fmt"
	"os"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/keychain"
	"github.com/0xdps/daemon-hound/internal/storage"
	"github.com/0xdps/daemon-hound/internal/utils"
	"github.com/spf13/cobra"
)

var rekeyCmd = &cobra.Command{
	Use:   "rekey",
	Short: "Change the master password used to protect your identity key",
	Long: `Re-encrypt the age identity key with a new master password.

The vault contents are not re-encrypted — only the local identity key wrapper
changes. You will need to enter your current password and then set a new one.`,
	RunE: runRekey,
}

func init() {
	rootCmd.AddCommand(rekeyCmd)
}

func runRekey(cmd *cobra.Command, args []string) error {
	cfg := config.NewConfig()
	if err := cfg.Load(); err != nil {
		return fmt.Errorf("not initialized: %w", err)
	}

	// Read current encrypted identity.
	encIdentity, err := os.ReadFile(config.IdentityPath())
	if err != nil {
		return fmt.Errorf("failed to read identity file: %w", err)
	}

	// Prompt for current password.
	current, err := utils.PromptPassword("Current master password:")
	if err != nil {
		return err
	}

	// Verify current password decrypts the identity (using global salt if available).
	identityBytes, err := utils.DecryptWithPassword(string(encIdentity), current, cfg.IdentitySalt())
	if err != nil {
		return fmt.Errorf("incorrect current password")
	}

	// Prompt for new password with confirmation.
	newPass, err := utils.PromptPassword("New master password:")
	if err != nil {
		return err
	}
	if newPass == "" {
		return fmt.Errorf("new password cannot be empty")
	}
	confirm, err := utils.PromptPassword("Confirm new master password:")
	if err != nil {
		return err
	}
	if newPass != confirm {
		return fmt.Errorf("passwords do not match")
	}

	// Re-encrypt identity with new password using existing global salt.
	newEncIdentity, err := utils.EncryptWithPassword(identityBytes, newPass, cfg.IdentitySalt())
	if err != nil {
		return fmt.Errorf("failed to re-encrypt identity: %w", err)
	}

	// Write atomically.
	tmp := config.IdentityPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(newEncIdentity), 0600); err != nil {
		return fmt.Errorf("failed to write new identity: %w", err)
	}
	if err := os.Rename(tmp, config.IdentityPath()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to commit new identity: %w", err)
	}

	// Update keychain if it was set.
	if keychain.IsSet() {
		if err := keychain.Store(newPass); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update keychain: %v\n", err)
		}
	}

	// Verify the new password actually works before reporting success.
	if _, err := storage.ParseIdentity(string(identityBytes)); err != nil {
		return fmt.Errorf("identity verification failed after re-key: %w", err)
	}

	fmt.Println("Master password updated successfully.")
	return nil
}

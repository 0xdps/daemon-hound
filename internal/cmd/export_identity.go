package cmd

import (
	"fmt"
	"os"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/utils"
	"github.com/spf13/cobra"
)

var exportIdentityCmd = &cobra.Command{
	Use:   "export-identity",
	Short: "Export your age private key (for setting up a new machine)",
	Long: `Decrypt and display your age private key in plaintext.

This is used when setting up DaemonHound on a new machine that should share
the same vault. Copy the printed key and use it with:

  dhd init --remote <url> --age-key <key>

on the new machine. Keep the key secret — anyone with it can decrypt your vault.`,
	RunE: runExportIdentity,
}

func init() {
	rootCmd.AddCommand(exportIdentityCmd)
}

func runExportIdentity(cmd *cobra.Command, args []string) error {
	cfg := config.NewConfig()
	if err := cfg.Load(); err != nil {
		return fmt.Errorf("not initialized: %w", err)
	}

	encIdentity, err := os.ReadFile(config.IdentityPath())
	if err != nil {
		return fmt.Errorf("failed to read identity file: %w", err)
	}

	password, err := utils.PromptPassword("Master password:")
	if err != nil {
		return err
	}

	identityBytes, err := utils.DecryptWithPassword(string(encIdentity), password, cfg.IdentitySalt())
	if err != nil {
		return fmt.Errorf("incorrect password")
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "⚠️  Keep this key secret. Anyone with it can decrypt your entire vault.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Println(string(identityBytes))
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "To use on a new machine:")
	fmt.Fprintf(os.Stderr, "  dhd init --remote %s --age-key <paste-key-above>\n", cfg.VaultRemote())
	return nil
}

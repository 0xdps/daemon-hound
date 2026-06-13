package cmd

import (
	"fmt"

	"github.com/0xdps/daemon-hound/internal/keychain"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove the stored master password from the OS keychain",
	Long: `Remove the cached master password from your OS keychain.

You will be prompted for your master password again on the next command.`,
	RunE: runLogout,
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}

func runLogout(cmd *cobra.Command, args []string) error {
	if err := keychain.Delete(); err != nil {
		return fmt.Errorf("failed to remove password from keychain: %w", err)
	}
	fmt.Println("Master password removed from keychain.")
	return nil
}

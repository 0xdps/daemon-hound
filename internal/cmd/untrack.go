package cmd

import (
	"fmt"

	"github.com/0xdps/daemon-hound/internal/audit"
	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/spf13/cobra"
)

var untrackCmd = &cobra.Command{
	Use:   "untrack <file>",
	Short: "Stop tracking a file",
	Long: `Stop tracking a file and remove it from the vault.

The local file is NOT deleted — only the encrypted copy in the vault
and the tracking metadata are removed.

Example:
  dh untrack .env.local`,
	Args: cobra.ExactArgs(1),
	RunE: runUntrack,
}

func init() {
	rootCmd.AddCommand(untrackCmd)
}

func runUntrack(cmd *cobra.Command, args []string) error {
	defer mustLock()()
	_, vault, tr, err := loadContext()
	if err != nil {
		return err
	}

	namespace, relPath, err := tr.ResolveKey(args[0])
	if err != nil {
		return fmt.Errorf("failed to resolve file key: %w", err)
	}
	key := namespace + ":" + relPath

	gitClient := git.NewClient(config.VaultPath())
	if err := vaultCommitPush(gitClient, vault, fmt.Sprintf("daemon-hound: untrack %s (%s)", relPath, namespace), func(state *models.VaultState) error {
		file, exists := state.Files[key]
		if !exists {
			return fmt.Errorf("file is not tracked: %s", args[0])
		}
		if err := vault.RemoveFile(file); err != nil {
			return fmt.Errorf("failed to remove file from vault: %w", err)
		}
		delete(state.Files, key)
		return nil
	}); err != nil {
		return err
	}

	fmt.Printf("Untracked: %s (%s)\n", args[0], namespace)
	fmt.Println("Note: the local file was not deleted.")
	audit.Log("untrack", fmt.Sprintf("%s:%s", namespace, relPath))
	return nil
}

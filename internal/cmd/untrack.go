package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/utils"
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
	_, vault, _, err := loadContext()
	if err != nil {
		return err
	}

	localPath, err := utils.NormalizePath(args[0])
	if err != nil {
		return err
	}

	// Determine namespace and relative path
	var namespace, relPath string
	if utils.IsInsideGitRepo(filepath.Dir(localPath)) {
		repoRoot, err := utils.FindGitRoot(filepath.Dir(localPath))
		if err != nil {
			return err
		}
		origin, err := utils.GetGitOrigin(repoRoot)
		if err != nil {
			return fmt.Errorf("failed to get git origin: %w", err)
		}
		namespace, err = utils.DeriveNamespace(origin)
		if err != nil {
			return fmt.Errorf("failed to derive namespace: %w", err)
		}
		relPath, err = utils.RelPath(repoRoot, localPath)
		if err != nil {
			return err
		}
	} else {
		namespace = "global"
		relPath = filepath.Base(localPath)
	}

	// Load vault state
	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load vault state: %w", err)
	}

	key := namespace + ":" + relPath
	file, exists := state.Files[key]
	if !exists {
		return fmt.Errorf("file is not tracked: %s", args[0])
	}

	// Remove from vault storage
	if err := vault.RemoveFile(file); err != nil {
		return fmt.Errorf("failed to remove file from vault: %w", err)
	}

	// Remove from state
	delete(state.Files, key)
	if err := vault.SaveState(state); err != nil {
		return fmt.Errorf("failed to save vault state: %w", err)
	}

	// Commit
	gitClient := git.NewClient(config.VaultPath())
	if err := gitClient.CommitAll(fmt.Sprintf("daemon-hound: untrack %s (%s)", relPath, namespace)); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Printf("Untracked: %s (%s)\n", args[0], namespace)
	fmt.Println("Note: the local file was not deleted.")
	return nil
}

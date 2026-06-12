package cmd

import (
	"fmt"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/sync"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync tracked files with the vault",
	Long: `Pull remote changes from the vault, then push local dirty files.

This is the main command for keeping your tracked files in sync across machines.`,
	RunE: runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, vault, tr, err := loadContext()
	if err != nil {
		return err
	}

	gitClient := git.NewClient(config.VaultPath())
	syncer := sync.NewSyncer(vault, tr, gitClient, cfg)

	results, err := syncer.Sync()
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Println("Everything is up to date.")
		return nil
	}

	for _, r := range results {
		if r.Error != nil {
			fmt.Printf("  %s/%s  error: %v\n", r.File.Namespace, r.File.RelPath, r.Error)
		} else {
			fmt.Printf("  %s/%s  %s\n", r.File.Namespace, r.File.RelPath, r.Action)
		}
	}
	return nil
}

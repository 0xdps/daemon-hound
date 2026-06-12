package cmd

import (
	"fmt"
	"os"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/0xdps/daemon-hound/internal/storage"
	"github.com/0xdps/daemon-hound/internal/tracker"
	"github.com/0xdps/daemon-hound/internal/utils"
	"github.com/spf13/cobra"
)

var trackMode string

var trackCmd = &cobra.Command{
	Use:   "track <file>",
	Short: "Start tracking a file",
	Long: `Track a file for syncing or backup.

DaemonHound reads the origin remote of the current Git repository to derive
a namespace, encrypts the file, and stores it in the vault.

Examples:
  dh track .env.local              # sync mode (default)
  dh track .env.test --mode sync   # explicit sync mode
  dh track ~/.zshrc --mode backup  # backup mode (this machine only)`,
	Args: cobra.ExactArgs(1),
	RunE: runTrack,
}

func init() {
	trackCmd.Flags().StringVar(&trackMode, "mode", "sync", "Tracking mode: sync or backup")
	rootCmd.AddCommand(trackCmd)
}

func runTrack(cmd *cobra.Command, args []string) error {
	_, vault, tr, err := loadContext()
	if err != nil {
		return err
	}

	mode := models.FileMode(trackMode)
	if mode != models.ModeSync && mode != models.ModeBackup {
		return fmt.Errorf("invalid mode: %s (must be 'sync' or 'backup')", trackMode)
	}

	file, err := tr.Track(args[0], mode)
	if err != nil {
		return err
	}

	// Update vault state
	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load vault state: %w", err)
	}
	key := file.Namespace + ":" + file.RelPath
	state.Files[key] = *file
	if err := vault.SaveState(state); err != nil {
		return fmt.Errorf("failed to save vault state: %w", err)
	}

	// Commit
	gitClient := git.NewClient(config.VaultPath())
	if err := gitClient.CommitAll(fmt.Sprintf("daemon-hound: track %s (%s)", file.RelPath, file.Namespace)); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Printf("Tracked: %s (%s, mode=%s)\n", args[0], file.Namespace, mode)
	return nil
}

// loadContext loads config, vault, and tracker. It prompts for the master password.
func loadContext() (*config.Config, *storage.Vault, *tracker.Tracker, error) {
	cfg := config.NewConfig()
	if err := cfg.Load(); err != nil {
		return nil, nil, nil, fmt.Errorf("not initialized: %w", err)
	}

	password, err := utils.PromptPassword("Master password:")
	if err != nil {
		return nil, nil, nil, err
	}

	encIdentity, err := os.ReadFile(config.IdentityPath())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read identity: %w", err)
	}
	identityStr, err := utils.DecryptWithPassword(string(encIdentity), password)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to decrypt identity (wrong password?): %w", err)
	}
	identity, err := storage.ParseIdentity(string(identityStr))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse identity (wrong password?): %w", err)
	}

	vault := storage.NewVault(config.VaultPath(), identity)
	tr := tracker.NewTracker(vault, cfg)
	return cfg, vault, tr, nil
}

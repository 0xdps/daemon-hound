package cmd

import (
	"fmt"
	"os"

	"github.com/0xdps/daemon-hound/internal/audit"
	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/keychain"
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
  dhd track .env.local              # sync mode (default)
  dhd track .env.test --mode sync   # explicit sync mode
  dhd track ~/.zshrc --mode backup  # backup mode (this machine only)`,
	Args: cobra.ExactArgs(1),
	RunE: runTrack,
}

func init() {
	trackCmd.Flags().StringVar(&trackMode, "mode", "sync", "Tracking mode: sync or backup")
	rootCmd.AddCommand(trackCmd)
}

func runTrack(cmd *cobra.Command, args []string) error {
	defer mustLock()()

	cfg, vault, tr, err := loadContext()
	if err != nil {
		return err
	}

	mode := models.FileMode(trackMode)
	if mode != models.ModeSync && mode != models.ModeBackup {
		return fmt.Errorf("invalid mode: %s (must be 'sync' or 'backup')", trackMode)
	}

	ns, rel, err := tr.ResolveKey(args[0])
	if err != nil {
		return fmt.Errorf("failed to resolve file key: %w", err)
	}
	key := ns + ":" + rel

	gitClient := git.NewClient(config.VaultPath())

	// Pull BEFORE tr.Track() writes the .age file to disk.
	// If we pull after, git sees the freshly-created .age file as an untracked
	// working-tree file that the remote also has and refuses to merge.
	if err := gitClient.Pull(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not pull before write (offline?): %v\n", err)
	}

	// Check idempotency against the (now up-to-date) state before encrypting.
	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load vault state: %w", err)
	}
	if existing, exists := state.Files[key]; exists {
		fmt.Printf("Already tracked: %s (%s, mode=%s)\n", args[0], existing.Namespace, existing.Mode)
		return nil
	}

	file, err := tr.Track(args[0], mode)
	if err != nil {
		return err
	}

	state.Files[key] = *file
	if err := vault.SaveState(state); err != nil {
		return fmt.Errorf("failed to save vault state: %w", err)
	}
	if err := gitClient.CommitAll(fmt.Sprintf("daemon-hound: track %s (%s)", file.RelPath, file.Namespace)); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	// Push with one pull-and-retry on non-fast-forward rejection.
	if err := gitClient.Push(); err != nil {
		if pullErr := gitClient.Pull(); pullErr != nil {
			return fmt.Errorf("failed to push: %w (and pull retry failed: %v)", err, pullErr)
		}
		if err := gitClient.Push(); err != nil {
			return fmt.Errorf("failed to push: %w", err)
		}
	}

	fmt.Printf("Tracked: %s (%s, mode=%s)\n", args[0], file.Namespace, mode)

	// Warn if the file is not gitignored — secrets should never be in git history.
	if ns != "global" {
		if repoRoot, ok := cfg.GetBinding(ns); ok {
			if utils.IsTrackedInGit(repoRoot, rel) {
				fmt.Fprintf(os.Stderr, "Warning: %s is tracked in git — its plaintext contents may be in git history\n", rel)
			} else if !utils.IsGitIgnored(repoRoot, rel) {
				fmt.Fprintf(os.Stderr, "Tip: consider adding %s to .gitignore\n", rel)
			}
		}
	}

	audit.Log("track", fmt.Sprintf("%s:%s (mode=%s)", ns, rel, mode))
	return nil
}

// loadContext loads config, vault, and tracker. It uses the keychain for the master password.
func loadContext() (*config.Config, *storage.Vault, *tracker.Tracker, error) {
	cfg := config.NewConfig()
	if err := cfg.Load(); err != nil {
		return nil, nil, nil, fmt.Errorf("not initialized: %w", err)
	}

	// Try keychain first
	password, err := keychain.Retrieve()
	if err != nil {
		// Not in keychain — prompt and store
		password, err = keychain.PromptAndStore(utils.PromptPassword)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	encIdentity, err := os.ReadFile(config.IdentityPath())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read identity: %w", err)
	}
	identityStr, err := utils.DecryptWithPassword(string(encIdentity), password, cfg.IdentitySalt())
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

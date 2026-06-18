package cmd

import (
	"fmt"
	"strings"

	"github.com/0xdps/daemon-hound/internal/audit"
	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/spf13/cobra"
)

var untrackLocal bool
var untrackMissing bool

var untrackCmd = &cobra.Command{
	Use:   "untrack <file>",
	Short: "Stop tracking a file",
	Long: `Stop tracking a file and remove it from the vault.

The local file is NOT deleted — only the encrypted copy in the vault
and the tracking metadata are removed.

Use --local to remove the file from this machine only. This keeps the
encrypted vault copy and shared tracking metadata intact for other machines.

Use --missing to ignore every tracked file that is currently missing on this
machine. This is useful after deleting local project folders.

Example:
  dh untrack .env.local
  dh untrack --local github.com/you/repo:.env.local
  dh untrack --missing`,
	Args: validateUntrackArgs,
	RunE: runUntrack,
}

func init() {
	untrackCmd.Flags().BoolVar(&untrackLocal, "local", false, "Remove tracking from this machine only; keep vault contents")
	untrackCmd.Flags().BoolVar(&untrackMissing, "missing", false, "Locally ignore all tracked files that are missing on this machine")
	rootCmd.AddCommand(untrackCmd)
}

func validateUntrackArgs(cmd *cobra.Command, args []string) error {
	if untrackMissing {
		return cobra.NoArgs(cmd, args)
	}
	return cobra.ExactArgs(1)(cmd, args)
}

func runUntrack(cmd *cobra.Command, args []string) error {
	defer mustLock()()
	cfg, vault, tr, err := loadContext()
	if err != nil {
		return err
	}

	if untrackMissing {
		return runUntrackMissing(cfg, vault, tr)
	}

	namespace, relPath, err := resolveUntrackKey(tr, args[0])
	if err != nil {
		return fmt.Errorf("failed to resolve file key: %w", err)
	}

	if untrackLocal {
		if err := cfg.IgnoreFile(namespace, relPath); err != nil {
			return fmt.Errorf("failed to save local ignore: %w", err)
		}
		fmt.Printf("Untracked locally: %s (%s)\n", relPath, namespace)
		fmt.Println("Note: vault contents were not removed; other machines are unaffected.")
		audit.Log("untrack local", fmt.Sprintf("%s:%s", namespace, relPath))
		return nil
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

func runUntrackMissing(cfg *config.Config, vault interface {
	LoadState() (*models.VaultState, error)
}, tr interface {
	Status(models.TrackedFile) (models.DirtyStatus, error)
}) error {
	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load vault state: %w", err)
	}

	ignored := 0
	for _, file := range state.Files {
		if file.Mode == models.ModeBackup && file.MachineID != cfg.MachineID() {
			continue
		}
		if cfg.IsFileIgnored(file.Namespace, file.RelPath) {
			continue
		}
		status, err := tr.Status(file)
		if err != nil {
			return fmt.Errorf("failed to check %s:%s: %w", file.Namespace, file.RelPath, err)
		}
		if status != models.StatusMissing {
			continue
		}
		if err := cfg.IgnoreFile(file.Namespace, file.RelPath); err != nil {
			return fmt.Errorf("failed to save local ignore: %w", err)
		}
		fmt.Printf("Untracked locally: %s (%s)\n", file.RelPath, file.Namespace)
		ignored++
	}

	if ignored == 0 {
		fmt.Println("No missing tracked files found for this machine.")
		return nil
	}

	fmt.Printf("Done. %d missing file(s) removed from this machine only.\n", ignored)
	fmt.Println("Vault contents were not removed; other machines are unaffected.")
	audit.Log("untrack missing", fmt.Sprintf("ignored:%d", ignored))
	return nil
}

func resolveUntrackKey(tr interface {
	ResolveKey(string) (string, string, error)
}, arg string) (string, string, error) {
	if namespace, relPath, ok := parseTrackedFileKey(arg); ok {
		return namespace, relPath, nil
	}
	return tr.ResolveKey(arg)
}

func parseTrackedFileKey(arg string) (string, string, bool) {
	namespace, relPath, ok := strings.Cut(arg, ":")
	if !ok || namespace == "" || relPath == "" {
		return "", "", false
	}
	return namespace, relPath, true
}

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/0xdps/daemon-hound/internal/storage"
	"github.com/spf13/cobra"
)

var readVaultDir string
var readVersion string

var readCmd = &cobra.Command{
	Use:   "read <file-or-secret>",
	Short: "Read a tracked file or secret from a cloned vault",
	Long: `Read and decrypt a tracked file or secret from a cloned vault.

This works with vaults created by 'dhd clone' — no machine initialization needed.

Arguments:
  <file>              Relative path of a tracked file (e.g. "github.com/you/repo:.env.local")
  secret:<name>        Name of a secret (e.g. "secret:openai-key")
  secret:<name>@v2    Specific version of a secret

Examples:
  dhd read github.com/you/repo:.env.local
  dhd read secret:openai-key
  dhd read secret:payment-config@v2
  dhd read --vault ./vault secret:stripe-key
  dhd read --version v1 secret:api-key`,
	Args: cobra.ExactArgs(1),
	RunE: runRead,
}

func init() {
	readCmd.Flags().StringVarP(&readVaultDir, "vault", "v", "", "Path to cloned vault directory (auto-detected if inside one)")
	readCmd.Flags().StringVar(&readVersion, "version", "", "Secret version to read (default: latest)")
	rootCmd.AddCommand(readCmd)
}

func runRead(cmd *cobra.Command, args []string) error {
	arg := args[0]

	// Resolve clone context
	ctx, err := loadCloneContext(readVaultDir)
	if err != nil {
		return err
	}

	vault, err := openClonedVault(ctx)
	if err != nil {
		return err
	}

	// Determine if reading a file or a secret
	if strings.HasPrefix(arg, "secret:") {
		return readSecret(vault, arg)
	}
	return readFile(vault, arg)
}

func readFile(vault *storage.Vault, arg string) error {
	// Parse namespace:relPath
	parts := strings.SplitN(arg, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid file key format: expected <namespace>:<relPath>, got %s", arg)
	}
	namespace, relPath := parts[0], parts[1]

	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("load vault state: %w", err)
	}

	key := namespace + ":" + relPath
	file, ok := state.Files[key]
	if !ok {
		return fmt.Errorf("file not found in vault: %s", arg)
	}

	// Determine vault path for the file
	var vaultFilePath string
	if file.Mode == models.ModeBackup {
		vaultFilePath = filepath.Join("backup", file.MachineID, file.Namespace, file.RelPath+".age")
	} else {
		vaultFilePath = filepath.Join("sync", file.Namespace, file.RelPath+".age")
	}

	// If file doesn't exist locally, pull it on demand
	fullPath := filepath.Join(vault.Path(), vaultFilePath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		gc := git.NewClient(vault.Path())
		fmt.Fprintf(os.Stderr, "Pulling %s...\n", vaultFilePath)
		if err := gc.PullFile(vaultFilePath); err != nil {
			return fmt.Errorf("pull file: %w", err)
		}
	}

	plaintext, err := vault.RetrieveFile(file)
	if err != nil {
		return fmt.Errorf("decrypt file: %w", err)
	}

	os.Stdout.Write(plaintext)
	return nil
}

func readSecret(vault *storage.Vault, arg string) error {
	// Strip "secret:" prefix
	name := strings.TrimPrefix(arg, "secret:")

	// Handle version suffix like secret:name@v2
	version := readVersion
	if idx := strings.LastIndex(name, "@"); idx >= 0 {
		version = name[idx+1:]
		name = name[:idx]
	}

	var value []byte

	// Try per-secret file first (new format).
	secretPath := filepath.Join("secrets", name+".toml.age")
	fullPath := filepath.Join(vault.Path(), secretPath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		// File not on disk — try pulling from git.
		gc := git.NewClient(vault.Path())
		fmt.Fprintf(os.Stderr, "Pulling %s...\n", secretPath)
		pullErr := gc.PullFile(secretPath)
		if pullErr != nil {
			// Per-secret file not in git. Fall back to legacy inline format:
			// the secret value lives inside state.toml.age itself.
			if version != "" {
				return fmt.Errorf("versioned secrets require per-secret files (not available in legacy format)")
			}
			v, legacyErr := vault.ReadLegacySecret(name)
			if legacyErr != nil {
				return fmt.Errorf("read secret: %w", legacyErr)
			}
			if v == nil {
				return fmt.Errorf("secret not found: %s", name)
			}
			value = v
			os.Stdout.Write(value)
			if len(value) > 0 && value[len(value)-1] != '\n' {
				fmt.Println()
			}
			return nil
		}
	}

	sf, err := vault.LoadSecretFile(name)
	if err != nil {
		return fmt.Errorf("load secret: %w", err)
	}

	if version != "" {
		value, err = getVersionValue(vault, sf, version)
		if err != nil {
			return fmt.Errorf("get version %s: %w", version, err)
		}
	} else {
		value, err = getLatestValue(vault, sf)
		if err != nil {
			return fmt.Errorf("get latest value: %w", err)
		}
	}

	os.Stdout.Write(value)
	if len(value) > 0 && value[len(value)-1] != '\n' {
		fmt.Println() // ensure trailing newline for terminal readability
	}
	return nil
}

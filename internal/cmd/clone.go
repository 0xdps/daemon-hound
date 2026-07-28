// Copyright (C) 2026 DaemonHound Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/storage"
	"github.com/0xdps/daemon-hound/internal/utils"
	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

var cloneDir string
var cloneIdentity string
var cloneNamespaces []string
var cloneSecrets []string

var cloneCmd = &cobra.Command{
	Use:   "clone <git-url> [directory]",
	Short: "Clone a vault repository without full machine initialization",
	Long: `Clone a DaemonHound vault repository for read-only or ad-hoc access.

Unlike 'dhd init', this does NOT:
  • Create a machine ID or ~/.dh/config.toml
  • Install a background daemon
  • Configure Git merge drivers or hooks
  • Store passwords in the system keychain

Partial clone:
  Use --namespace and --secret to download only specific files.
  Git sparse-checkout keeps everything else absent from the working tree.

Examples:
  dhd clone git@github.com:you/vault.git
  dhd clone git@github.com:you/vault.git ./my-vault
  dhd clone git@github.com:you/vault.git --identity ./identity.age

  # Partial clone — only specific namespaces and secrets
  dhd clone git@github.com:you/vault.git --namespace github.com/you/repo
  dhd clone git@github.com:you/vault.git --secret payment-key,jwt-secret
  dhd clone git@github.com:you/vault.git --namespace github.com/you/repo --secret stripe-key

After cloning, use 'dhd read' to access files and secrets.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runClone,
}

func init() {
	cloneCmd.Flags().StringVarP(&cloneDir, "directory", "d", "", "Target directory (default: ./vault)")
	cloneCmd.Flags().StringVarP(&cloneIdentity, "identity", "i", "", "Path to age identity file (default: prompt for raw key)")
	cloneCmd.Flags().StringSliceVarP(&cloneNamespaces, "namespace", "n", nil, "Comma-separated namespaces to checkout (e.g. 'github.com/you/repo,github.com/you/other')")
	cloneCmd.Flags().StringSliceVarP(&cloneSecrets, "secret", "s", nil, "Comma-separated secret names to checkout (e.g. 'payment-key,jwt-secret')")
	rootCmd.AddCommand(cloneCmd)
}

// CloneContext stores the minimal metadata needed for a cloned vault.
type CloneContext struct {
	VaultPath    string `toml:"vault_path"`
	IdentityPath string `toml:"identity_path"`
	RemoteURL    string `toml:"remote_url"`
}

func runClone(cmd *cobra.Command, args []string) error {
	remoteURL := args[0]

	targetDir := cloneDir
	if targetDir == "" {
		if len(args) >= 2 {
			targetDir = args[1]
		} else {
			targetDir = "vault"
		}
	}

	absDir, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("resolve target directory: %w", err)
	}

	// Clone with blob:none partial clone. Only the vault index downloads
	// initially. All other files pull on demand via 'dhd read'.
	initialPaths := buildInitialPaths(cloneNamespaces, cloneSecrets)

	fmt.Fprintf(os.Stderr, "Cloning %s into %s...\n", remoteURL, absDir)
	fmt.Fprintln(os.Stderr, "  (partial clone — on-demand fetch)")
	if err := git.CloneSparse(remoteURL, absDir, initialPaths); err != nil {
		return fmt.Errorf("clone failed: %w", err)
	}
	fmt.Fprintln(os.Stderr, "✓ Clone complete")

	// Resolve identity
	var identityPath string
	if cloneIdentity != "" {
		identityPath, err = filepath.Abs(cloneIdentity)
		if err != nil {
			return fmt.Errorf("resolve identity path: %w", err)
		}
		if _, err := os.Stat(identityPath); err != nil {
			return fmt.Errorf("identity file not found: %s", identityPath)
		}
	} else {
		// Prompt for raw age key and write to a file in the vault directory
		key, err := utils.PromptPassword("Enter age identity key (AGE-SECRET-KEY-...):")
		if err != nil {
			return err
		}
		if key == "" {
			return fmt.Errorf("identity key cannot be empty")
		}
		// Validate it parses
		if _, err := storage.ParseIdentity(key); err != nil {
			return fmt.Errorf("invalid age identity: %w", err)
		}
		identityPath = filepath.Join(absDir, ".dhd", "identity.age")
		if err := os.MkdirAll(filepath.Dir(identityPath), 0700); err != nil {
			return fmt.Errorf("create .dhd directory: %w", err)
		}
		if err := os.WriteFile(identityPath, []byte(key), 0600); err != nil {
			return fmt.Errorf("write identity file: %w", err)
		}
		fmt.Fprintf(os.Stderr, "✓ Identity saved to %s\n", identityPath)
	}

	// Verify the identity can decrypt the vault state.
	// Use a raw decrypt instead of vault.LoadState() to avoid triggering
	// legacy secret migration, which would write all secret files to disk.
	identityBytes, err := os.ReadFile(identityPath)
	if err != nil {
		return fmt.Errorf("read identity: %w", err)
	}
	identity, err := storage.ParseIdentity(string(identityBytes))
	if err != nil {
		return fmt.Errorf("parse identity: %w", err)
	}

	enc, err := os.ReadFile(filepath.Join(absDir, "state.toml.age"))
	if err != nil {
		return fmt.Errorf("read vault state: %w", err)
	}
	vault := storage.NewVault(absDir, identity)
	if _, err := vault.Decrypt(enc); err != nil {
		return fmt.Errorf("cannot decrypt vault state — wrong identity? %w", err)
	}
	fmt.Fprintln(os.Stderr, "✓ Vault state decrypted successfully")

	// Write clone context so 'dhd read' knows where to look
	ctx := CloneContext{
		VaultPath:    absDir,
		IdentityPath: identityPath,
		RemoteURL:    remoteURL,
	}
	ctxPath := filepath.Join(absDir, ".dhd", "clone.toml")
	if err := writeCloneContext(ctxPath, ctx); err != nil {
		return fmt.Errorf("write clone context: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nVault ready at: %s\n", absDir)
	fmt.Fprintln(os.Stderr, "\nUsage:")
	fmt.Fprintf(os.Stderr, "  dhd read --vault %s <file>\n", absDir)
	fmt.Fprintf(os.Stderr, "  dhd read --vault %s secret:<name>\n", absDir)
	fmt.Fprintln(os.Stderr, "\nOr cd into the directory and run:")
	fmt.Fprintln(os.Stderr, "  dhd read <file>")
	fmt.Fprintln(os.Stderr, "  dhd read secret:<name>")
	return nil
}

func writeCloneContext(path string, ctx CloneContext) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(ctx)
}

// loadCloneContext reads clone context from the given directory or walks up
// from cwd looking for a .dhd/clone.toml file.
func loadCloneContext(dir string) (*CloneContext, error) {
	if dir != "" {
		path := filepath.Join(dir, ".dhd", "clone.toml")
		return readCloneContext(path)
	}

	// Walk up from cwd looking for .dhd/clone.toml
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for {
		path := filepath.Join(cwd, ".dhd", "clone.toml")
		if ctx, err := readCloneContext(path); err == nil {
			return ctx, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}

	return nil, fmt.Errorf("no clone context found — run 'dhd clone' first or use --vault")
}

func readCloneContext(path string) (*CloneContext, error) {
	var ctx CloneContext
	if _, err := toml.DecodeFile(path, &ctx); err != nil {
		return nil, err
	}
	if ctx.VaultPath == "" {
		return nil, fmt.Errorf("invalid clone context")
	}
	return &ctx, nil
}

// openClonedVault loads a vault from a clone context.
func openClonedVault(ctx *CloneContext) (*storage.Vault, error) {
	identityBytes, err := os.ReadFile(ctx.IdentityPath)
	if err != nil {
		return nil, fmt.Errorf("read identity: %w", err)
	}
	identity, err := storage.ParseIdentity(string(identityBytes))
	if err != nil {
		return nil, fmt.Errorf("parse identity: %w", err)
	}
	return storage.NewVault(ctx.VaultPath, identity), nil
}

// buildInitialPaths converts namespace and secret flags into git checkout paths.
// Namespaces map to sync/ directories; secrets map to secrets/*.toml.age files.
// Note: CloneSparse always includes state.toml.age, so it's not needed here.
func buildInitialPaths(namespaces, secrets []string) []string {
	var paths []string
	for _, ns := range namespaces {
		paths = append(paths, filepath.Join("sync", ns)+"/")
	}
	for _, name := range secrets {
		paths = append(paths, filepath.Join("secrets", name+".toml.age"))
	}
	return paths
}

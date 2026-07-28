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
	"strings"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/merge"
	"github.com/spf13/cobra"
)

var mergeCmd = &cobra.Command{
	Use:   "merge <base> <local> <remote> [path]",
	Short: "Git merge driver for DaemonHound vault files",
	Long: `Acts as a Git merge driver for encrypted and structured files in the vault.

Git calls this command during merge/rebase/cherry-pick as:
  dhd merge %O %A %B %P

Arguments:
  <base>   Path to the base (common ancestor) version of the file
  <local>  Path to the local (current branch) version — result is written here
  <remote> Path to the remote (other branch) version
  [path]   Original pathname of the file being merged (relative to repo root).
           If omitted, inferred from the current working directory.

Exit codes:
  0  — merge succeeded (result written to <local>)
  1  — merge failed or unsupported file type (Git will mark conflict)`,
	Args: cobra.RangeArgs(3, 4),
	RunE: runMerge,
}

func init() {
	rootCmd.AddCommand(mergeCmd)
}

func runMerge(cmd *cobra.Command, args []string) error {
	basePath := args[0]
	localPath := args[1]
	remotePath := args[2]

	// Determine the filename for driver matching.
	// Git passes %P as the pathname relative to the repository root.
	var filename string
	if len(args) >= 4 {
		filename = args[3]
	} else {
		filename = inferFilename(localPath)
	}

	// Read the three versions.
	base, err := os.ReadFile(basePath)
	if err != nil {
		return fmt.Errorf("read base: %w", err)
	}
	local, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("read local: %w", err)
	}
	remote, err := os.ReadFile(remotePath)
	if err != nil {
		return fmt.Errorf("read remote: %w", err)
	}

	// Determine which registry to use.
	var merger *merge.Registry
	if isEncryptedVaultFile(filename) {
		// Encrypted vault files need the identity to decrypt before merging.
		vaultMerger, err := buildVaultMerger()
		if err != nil {
			// Cannot load identity — don't let Git corrupt the encrypted file
			// with text conflict markers. Keep local and report success so Git
			// doesn't mark it conflicted. The next sync will retry.
			fmt.Fprintf(os.Stderr, "daemon-hound merge: %v (keeping local version)\n", err)
			return nil
		}
		merger = vaultMerger
	} else {
		// Non-encrypted files use the basic registry (env, json, csv, text).
		merger = merge.NewRegistry()
	}

	// Attempt merge.
	merged, result, err := merger.Resolve(filename, base, local, remote)
	if err != nil {
		return fmt.Errorf("merge error: %w", err)
	}

	switch result {
	case merge.Merged:
		if err := os.WriteFile(localPath, merged, 0644); err != nil {
			return fmt.Errorf("write merged result: %w", err)
		}
		return nil // exit 0 = success

	case merge.HasConflict:
		// For encrypted vault files, letting Git mark it conflicted would
		// insert text conflict markers into binary ciphertext, corrupting it.
		// Write the local version and exit 0 to prevent Git damage.
		// The daemon/syncer will detect and resolve via smart merge later.
		if isEncryptedVaultFile(filename) {
			fmt.Fprintf(os.Stderr, "daemon-hound merge: conflict in %s — keeping local version\n", filename)
			return nil // Keep local as-is
		}
		// For plain-text files, exit 1 so Git marks it conflicted normally.
		return fmt.Errorf("merge conflict in %s", filename)

	case merge.Unsupported:
		// No driver can handle this file. Exit 1 so Git tries its default merge.
		return fmt.Errorf("unsupported file: %s", filename)
	}

	return nil
}

// inferFilename attempts to determine the vault-relative filename from the
// temporary file path Git provides. Git creates temp files with random names
// in .git/ directory, so we use the current working directory for context.
func inferFilename(localPath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Base(localPath)
	}

	vaultPath := config.VaultPath()
	secretsDir := filepath.Join(vaultPath, "secrets")

	// If we're inside the vault, use the directory context.
	if cwd == secretsDir || strings.HasPrefix(cwd, secretsDir+string(filepath.Separator)) {
		return "secrets/" + filepath.Base(localPath)
	}
	if cwd == vaultPath || strings.HasPrefix(cwd, vaultPath+string(filepath.Separator)) {
		return filepath.Base(localPath)
	}

	// Default: just the basename.
	return filepath.Base(localPath)
}

// isEncryptedVaultFile reports whether the filename indicates an encrypted
// vault file that requires the age identity to decrypt.
func isEncryptedVaultFile(filename string) bool {
	return filename == "state.toml.age" ||
		(strings.HasPrefix(filename, "secrets/") && strings.HasSuffix(filename, ".toml.age"))
}

// buildVaultMerger loads the vault identity and returns a merge registry
// configured with the SecretDriver for decrypting/merging/re-encrypting.
func buildVaultMerger() (*merge.Registry, error) {
	vault, err := loadVault()
	if err != nil {
		return nil, err
	}
	return merge.NewRegistry().WithSecretDriver(vault.Encrypt, vault.Decrypt), nil
}

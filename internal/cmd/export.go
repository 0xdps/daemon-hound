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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/0xdps/daemon-hound/internal/storage"
	"github.com/0xdps/daemon-hound/internal/utils"
	"github.com/spf13/cobra"
)

var exportDir string

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Decrypt and export all tracked files and secrets to a local directory",
	Long: `Decrypt every tracked file and secret from the vault and write them
to a local directory in plaintext. Useful for migration, auditing, or
disaster recovery.

The output directory structure mirrors the vault layout:
  <dir>/files/<namespace>/<relPath>
  <dir>/secrets.json

WARNING: the export directory contains unencrypted secrets. Handle with care.`,
	RunE: runExport,
}

func init() {
	exportCmd.Flags().StringVar(&exportDir, "dir", "", "Output directory (default: ./dhd-export-<timestamp>)")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	cfg := config.NewConfig()
	if err := cfg.Load(); err != nil {
		return fmt.Errorf("not initialized: %w", err)
	}

	// Always require explicit password confirmation for export — never use keychain.
	fmt.Fprintln(os.Stderr, "Export contains unencrypted secrets. Master password required to proceed.")
	password, err := utils.PromptPassword("Master password:")
	if err != nil {
		return err
	}

	encIdentity, err := os.ReadFile(config.IdentityPath())
	if err != nil {
		return fmt.Errorf("failed to read identity: %w", err)
	}
	identityStr, err := utils.DecryptWithPassword(string(encIdentity), password, cfg.IdentitySalt())
	if err != nil {
		return fmt.Errorf("incorrect password")
	}
	identity, err := storage.ParseIdentity(string(identityStr))
	if err != nil {
		return fmt.Errorf("failed to parse identity: %w", err)
	}

	vault := storage.NewVault(config.VaultPath(), identity)

	if exportDir == "" {
		exportDir = fmt.Sprintf("dhd-export-%s", time.Now().UTC().Format("20060102-150405"))
	}

	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load vault state: %w", err)
	}

	// Export tracked files.
	fileCount := 0
	for _, file := range state.Files {
		if !vault.FileExistsInVault(file) {
			fmt.Fprintf(os.Stderr, "  skipped (not in vault): %s:%s\n", file.Namespace, file.RelPath)
			continue
		}

		plaintext, err := vault.RetrieveFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error decrypting %s:%s: %v\n", file.Namespace, file.RelPath, err)
			continue
		}

		dest := filepath.Join(exportDir, "files", file.Namespace, file.RelPath)
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return fmt.Errorf("failed to create export directory: %w", err)
		}
		if err := os.WriteFile(dest, plaintext, 0600); err != nil {
			return fmt.Errorf("failed to write export file: %w", err)
		}
		fmt.Printf("  exported: %s/%s\n", file.Namespace, file.RelPath)
		fileCount++
	}

	// Export secrets.
	type secretExport struct {
		Name     string             `json:"name"`
		Latest   string             `json:"latest"`
		Versions map[string]string  `json:"versions"` // version -> reason
		Value    string             `json:"value"`
		Refs     []models.SecretRef `json:"refs,omitempty"`
	}
	var secrets []secretExport

	names, err := vault.ListSecretNames()
	if err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}
	for _, name := range names {
		sf, err := vault.LoadSecretFile(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error loading secret %s: %v\n", name, err)
			continue
		}
		val, err := getLatestValue(vault, sf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error decrypting secret %s: %v\n", name, err)
			continue
		}
		versions := make(map[string]string)
		for v, ver := range sf.Versions {
			versions[v] = ver.Reason
		}
		secrets = append(secrets, secretExport{
			Name:     name,
			Latest:   sf.Latest,
			Versions: versions,
			Value:    string(val),
			Refs:     sf.Refs,
		})
		fmt.Printf("  exported secret: %s\n", name)
	}

	if len(secrets) > 0 {
		secretsPath := filepath.Join(exportDir, "secrets.json")
		if err := os.MkdirAll(exportDir, 0700); err != nil {
			return err
		}
		data, _ := json.MarshalIndent(secrets, "", "  ")
		if err := os.WriteFile(secretsPath, data, 0600); err != nil {
			return fmt.Errorf("failed to write secrets export: %w", err)
		}
	}

	fmt.Printf("\nExported %d file(s) and %d secret(s) to %s\n", fileCount, len(secrets), exportDir)
	fmt.Println("WARNING: this directory contains unencrypted secrets. Delete it when done.")
	return nil
}

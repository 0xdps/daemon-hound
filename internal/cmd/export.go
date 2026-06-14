package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/0xdps/daemon-hound/internal/models"
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
	exportCmd.Flags().StringVar(&exportDir, "dir", "", "Output directory (default: ./dh-export-<timestamp>)")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	_, vault, _, err := loadContext()
	if err != nil {
		return err
	}

	if exportDir == "" {
		exportDir = fmt.Sprintf("dh-export-%s", time.Now().UTC().Format("20060102-150405"))
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
		Name  string             `json:"name"`
		Value string             `json:"value"`
		Refs  []models.SecretRef `json:"refs,omitempty"`
	}
	var secrets []secretExport
	for name, secret := range state.Secrets {
		val, err := vault.Decrypt(secret.Value)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error decrypting secret %s: %v\n", name, err)
			continue
		}
		secrets = append(secrets, secretExport{
			Name:  name,
			Value: string(val),
			Refs:  secret.Refs,
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

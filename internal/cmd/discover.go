package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/sync"
	"github.com/0xdps/daemon-hound/internal/utils"
	"github.com/spf13/cobra"
)

var discoverDepth int

var discoverCmd = &cobra.Command{
	Use:   "discover [path]",
	Short: "Scan directories for Git repos and sync known namespaces",
	Long: `Scan a directory tree for Git repositories and automatically sync
any namespaces that already exist in the vault.

Example:
  dh discover           # scan current directory
  dh discover ~/projects  # scan ~/projects`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDiscover,
}

func init() {
	discoverCmd.Flags().IntVar(&discoverDepth, "depth", 4, "Maximum directory depth to scan")
	rootCmd.AddCommand(discoverCmd)
}

func runDiscover(cmd *cobra.Command, args []string) error {
	scanPath := "."
	if len(args) > 0 {
		scanPath = args[0]
	}
	scanPath, err := utils.NormalizePath(scanPath)
	if err != nil {
		return err
	}

	cfg, vault, tr, err := loadContext()
	if err != nil {
		return err
	}

	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load vault state: %w", err)
	}

	// Build set of known namespaces
	knownNS := make(map[string]bool)
	for _, f := range state.Files {
		knownNS[f.Namespace] = true
	}

	fmt.Printf("Scanning %s (depth %d)...\n\n", scanPath, discoverDepth)

	var found []struct {
		ns     string
		root   string
		status string
	}

	err = filepath.WalkDir(scanPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" {
			return nil
		}

		// Check depth
		rel, _ := filepath.Rel(scanPath, path)
		depth := 0
		for _, c := range rel {
			if c == filepath.Separator {
				depth++
			}
		}
		if depth > discoverDepth {
			return filepath.SkipDir
		}

		// Check if this is a git repo
		gitDir := filepath.Join(path, ".git")
		if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
			return nil
		}

		origin, err := utils.GetGitOrigin(path)
		if err != nil {
			return nil
		}
		ns, err := utils.DeriveNamespace(origin)
		if err != nil {
			return nil
		}

		if !knownNS[ns] {
			found = append(found, struct {
				ns     string
				root   string
				status string
			}{ns, path, "not in vault"})
			return filepath.SkipDir
		}

		// Record binding and sync
		if err := cfg.SetBinding(ns, path); err != nil {
			found = append(found, struct {
				ns     string
				root   string
				status string
			}{ns, path, "binding error"})
			return filepath.SkipDir
		}

		gitClient := git.NewClient(config.VaultPath())
		syncer := sync.NewSyncer(vault, tr, gitClient, cfg)
		results, err := syncer.Sync()
		if err != nil {
			found = append(found, struct {
				ns     string
				root   string
				status string
			}{ns, path, "sync error: " + err.Error()})
		} else {
			found = append(found, struct {
				ns     string
				root   string
				status string
			}{ns, path, fmt.Sprintf("%d files synced", len(results))})
		}

		return filepath.SkipDir
	})
	if err != nil {
		return err
	}

	for _, f := range found {
		fmt.Printf("%-40s  %s\n", f.ns, f.status)
	}
	return nil
}

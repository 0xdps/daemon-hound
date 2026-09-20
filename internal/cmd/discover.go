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
	"strings"
	"time"

	"github.com/0xdps/daemon-hound/internal/audit"
	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/output"
	"github.com/0xdps/daemon-hound/internal/sync"
	"github.com/0xdps/daemon-hound/internal/utils"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var discoverDepth int
var discoverOutput string

var discoverCmd = &cobra.Command{
	Use:   "discover [path]",
	Short: "Scan directories for Git repos and sync known namespaces",
	Long: `Scan a directory tree for Git repositories and automatically sync
any namespaces that already exist in the vault.

Example:
  dhd discover           # scan current directory
  dhd discover ~/projects  # scan ~/projects`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDiscover,
}

func init() {
	discoverCmd.Flags().IntVar(&discoverDepth, "depth", 4, "Maximum directory depth to scan")
	discoverCmd.Flags().StringVar(&discoverOutput, "output", "table", "Output format: table or json")
	rootCmd.AddCommand(discoverCmd)
}

func runDiscover(cmd *cobra.Command, args []string) error {
	defer mustLock()()
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

	showProgress := discoverOutput != "json" && term.IsTerminal(int(os.Stderr.Fd()))
	if discoverOutput != "json" {
		fmt.Fprintf(os.Stderr, "Scanning %s (depth %d)...\n", scanPath, discoverDepth)
	}

	lastProgress := time.Time{}
	repos, err := utils.FindGitRepos(scanPath, discoverDepth, func(p utils.ScanProgress) {
		if !showProgress {
			return
		}
		now := time.Now()
		if now.Sub(lastProgress) < 80*time.Millisecond && p.DirsVisited%50 != 0 {
			return
		}
		lastProgress = now
		rel := p.Current
		if r, err := filepath.Rel(scanPath, p.Current); err == nil {
			rel = r
		}
		fmt.Fprintf(os.Stderr, "\r  scanned %d dirs, found %d repos  %s\033[K", p.DirsVisited, p.ReposFound, truncatePath(rel, 60))
	})
	if err != nil {
		return err
	}
	if showProgress {
		fmt.Fprintf(os.Stderr, "\r  scanned complete — %d git repo(s) found\033[K\n\n", len(repos))
	} else if discoverOutput != "json" {
		fmt.Fprintf(os.Stderr, "Found %d git repo(s).\n\n", len(repos))
	}

	type discoverEntry struct {
		ns     string
		root   string
		status string
	}
	var found []discoverEntry
	bound := 0

	for _, repo := range repos {
		if !knownNS[repo.Namespace] {
			found = append(found, discoverEntry{repo.Namespace, repo.Root, "not in vault"})
			continue
		}
		if err := cfg.SetBinding(repo.Namespace, repo.Root); err != nil {
			found = append(found, discoverEntry{repo.Namespace, repo.Root, "binding error"})
			continue
		}
		bound++
		fileCount := 0
		for _, f := range state.Files {
			if f.Namespace == repo.Namespace {
				fileCount++
			}
		}
		noun := "file"
		if fileCount != 1 {
			noun = "files"
		}
		found = append(found, discoverEntry{
			ns:     repo.Namespace,
			root:   repo.Root,
			status: fmt.Sprintf("%d tracked %s", fileCount, noun),
		})
	}

	if bound > 0 {
		if showProgress {
			fmt.Fprintf(os.Stderr, "Syncing vault for %d bound namespace(s)...\n", bound)
		}
		gitClient := git.NewClient(config.VaultPath())
		syncer := sync.NewSyncer(vault, tr, gitClient, cfg)
		if _, syncErr := syncer.Sync(); syncErr != nil {
			for i, f := range found {
				if f.status != "not in vault" && f.status != "binding error" {
					found[i].status = "sync error: " + syncErr.Error()
				}
			}
		} else {
			for i, f := range found {
				if f.status != "not in vault" && f.status != "binding error" {
					found[i].status = f.status + "  ✓ synced"
				}
			}
		}
	}

	if discoverOutput == "json" {
		type jsonEntry struct {
			Namespace string `json:"namespace"`
			Root      string `json:"root"`
			Status    string `json:"status"`
		}
		var entries []jsonEntry
		for _, f := range found {
			entries = append(entries, jsonEntry{f.ns, f.root, f.status})
		}
		data, _ := json.MarshalIndent(entries, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	for _, f := range found {
		statusStr := f.status
		if f.status == "not in vault" {
			statusStr = output.Dim(f.status)
		} else if f.root != "" {
			statusStr = output.Green(f.status)
		}
		fmt.Printf("%-40s  %s\n", f.ns, statusStr)
	}
	synced := 0
	for _, f := range found {
		if f.root != "" && f.status != "not in vault" {
			synced++
		}
	}
	audit.Log("discover", fmt.Sprintf("path=%s synced=%d", scanPath, synced))
	return nil
}

func truncatePath(p string, max int) string {
	p = strings.ReplaceAll(p, "\n", " ")
	if max < 4 || len(p) <= max {
		return p
	}
	return "…" + p[len(p)-(max-1):]
}

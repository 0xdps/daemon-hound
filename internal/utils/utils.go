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

package utils

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// DefaultScanDepth is used when locating a repo for an unbound namespace.
// Matches a typical ~/src/github.com/owner/repo layout.
const DefaultScanDepth = 6

var (
	repoIndexMu   sync.Mutex
	repoIndex     map[string]string
	repoIndexHome string
	repoIndexAt   time.Time
)

// GitRepo is a Git working tree found by FindGitRepos.
type GitRepo struct {
	Root      string
	Namespace string
}

// ScanProgress reports live status while FindGitRepos walks a tree.
type ScanProgress struct {
	DirsVisited int
	ReposFound  int
	Current     string
}

// skipScanDirs are high-fanout or non-project directories that must not be
// descended into during repo discovery. The scan root itself is never skipped.
var skipScanDirs = map[string]struct{}{
	".git":             {},
	"node_modules":     {},
	"vendor":           {},
	"target":           {},
	"coverage":         {},
	"Pods":             {},
	"Carthage":         {},
	"bower_components": {},
	"__pycache__":      {},
	".venv":            {},
	"venv":             {},
	".tox":             {},
	".mypy_cache":      {},
	".pytest_cache":    {},
	".next":            {},
	".nuxt":            {},
	".output":          {},
	".turbo":           {},
	".parcel-cache":    {},
	"Library":          {},
	"Applications":     {},
	"Movies":           {},
	"Music":            {},
	"Pictures":         {},
	"Caches":           {},
	"DerivedData":      {},
	".Trash":           {},
	".cache":           {},
	".config":          {},
	".local":           {},
	".npm":             {},
	".yarn":            {},
	".pnpm-store":      {},
	".cargo":           {},
	".rustup":          {},
	".pyenv":           {},
	".nvm":             {},
	".sdkman":          {},
	".docker":          {},
	".gradle":          {},
	".m2":              {},
}

// ShouldSkipScanDir reports whether a directory name should be skipped while
// looking for Git repositories. The scan root is never passed here.
func ShouldSkipScanDir(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	_, ok := skipScanDirs[name]
	return ok
}

// IsGitRepo reports whether path is a Git working tree (.git dir or file).
func IsGitRepo(path string) bool {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

// FindGitRepos walks root for Git repositories up to maxDepth (same meaning as
// `dhd discover --depth`: number of path separators below root). It only reads
// directories, does not follow symlinks, and skips dependency/cache trees.
// progress may be nil.
func FindGitRepos(root string, maxDepth int, progress func(ScanProgress)) ([]GitRepo, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", root)
	}

	var repos []GitRepo
	visited := 0

	var walk func(dir string) error
	walk = func(dir string) error {
		visited++
		if progress != nil {
			progress(ScanProgress{DirsVisited: visited, ReposFound: len(repos), Current: dir})
		}

		rel, err := filepath.Rel(root, dir)
		if err != nil {
			return nil
		}
		depth := 0
		if rel != "." {
			depth = strings.Count(rel, string(filepath.Separator))
		}
		if depth > maxDepth {
			return nil
		}

		if IsGitRepo(dir) {
			origin, err := GetGitOrigin(dir)
			if err == nil {
				if ns, err := DeriveNamespace(origin); err == nil {
					repos = append(repos, GitRepo{Root: dir, Namespace: ns})
				}
			}
			return nil
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		for _, e := range entries {
			if !e.IsDir() || e.Type()&os.ModeSymlink != 0 {
				continue
			}
			child := filepath.Join(dir, e.Name())
			if ShouldSkipScanDir(e.Name()) {
				continue
			}
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk(root); err != nil {
		return nil, err
	}
	return repos, nil
}

// FindRepoRootForNamespace locates a local Git working tree whose origin
// matches namespace. The first scan of $HOME is cached for the process so
// status/sync/daemon do not re-walk the tree for every unbound namespace.
func FindRepoRootForNamespace(namespace string) (string, error) {
	if namespace == "" || namespace == "global" {
		return "", fmt.Errorf("no repo root found for namespace %s", namespace)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	repoIndexMu.Lock()
	defer repoIndexMu.Unlock()

	if repoIndex == nil || repoIndexHome != home || time.Since(repoIndexAt) > 5*time.Minute {
		repos, err := FindGitRepos(home, DefaultScanDepth, nil)
		if err != nil {
			return "", err
		}
		index := make(map[string]string, len(repos))
		for _, r := range repos {
			if _, ok := index[r.Namespace]; !ok {
				index[r.Namespace] = r.Root
			}
		}
		repoIndex = index
		repoIndexHome = home
		repoIndexAt = time.Now()
	}

	root, ok := repoIndex[namespace]
	if !ok {
		return "", fmt.Errorf("no repo root found for namespace %s", namespace)
	}
	return root, nil
}

// ResetRepoIndexForTest clears the process-wide repo scan cache.
func ResetRepoIndexForTest() {
	repoIndexMu.Lock()
	defer repoIndexMu.Unlock()
	repoIndex = nil
	repoIndexHome = ""
	repoIndexAt = time.Time{}
}

// DeriveNamespace extracts a namespace from a Git remote URL.
// Supported formats:
//
//	git@github.com:owner/repo.git      -> github.com/owner/repo
//	https://github.com/owner/repo.git  -> github.com/owner/repo
//	https://github.com/owner/repo      -> github.com/owner/repo
func DeriveNamespace(remoteURL string) (string, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return "", fmt.Errorf("empty remote URL")
	}

	// SSH format: git@host:path/to/repo.git
	if strings.HasPrefix(remoteURL, "git@") {
		parts := strings.SplitN(remoteURL, ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid SSH remote URL: %s", remoteURL)
		}
		host := strings.TrimPrefix(parts[0], "git@")
		path := strings.TrimSuffix(parts[1], ".git")
		return fmt.Sprintf("%s/%s", host, path), nil
	}

	// HTTPS format: https://host/path/to/repo.git
	if strings.HasPrefix(remoteURL, "http://") || strings.HasPrefix(remoteURL, "https://") {
		remoteURL = strings.TrimPrefix(remoteURL, "https://")
		remoteURL = strings.TrimPrefix(remoteURL, "http://")
		remoteURL = strings.TrimSuffix(remoteURL, ".git")
		return remoteURL, nil
	}

	return "", fmt.Errorf("unsupported remote URL format: %s", remoteURL)
}

// PromptPassword reads a password from the terminal without echoing.
func PromptPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt+" ")
	bytePassword, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}
	return string(bytePassword), nil
}

// PromptInput reads a line of input from stdin.
func PromptInput(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt+" ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	return strings.TrimSpace(input), nil
}

// FileChecksum returns the SHA-256 hex checksum of a file's contents.
func FileChecksum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// NormalizePath returns the absolute, cleaned path.
func NormalizePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// RelPath returns the relative path of target from base.
func RelPath(base, target string) (string, error) {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", fmt.Errorf("failed to compute relative path: %w", err)
	}
	return rel, nil
}

// IsInsideGitRepo checks if the given path is inside a Git repository.
func IsInsideGitRepo(path string) bool {
	for {
		gitDir := filepath.Join(path, ".git")
		info, err := os.Stat(gitDir)
		if err == nil && info.IsDir() {
			return true
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	return false
}

// FindGitRoot walks up from path to find the repository root containing .git.
func FindGitRoot(path string) (string, error) {
	path, err := NormalizePath(path)
	if err != nil {
		return "", err
	}
	for {
		gitDir := filepath.Join(path, ".git")
		info, err := os.Stat(gitDir)
		if err == nil && info.IsDir() {
			return path, nil
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", fmt.Errorf("not inside a git repository")
		}
		path = parent
	}
}

// GetGitOrigin returns the origin remote URL for the Git repo at the given path.
func GetGitOrigin(repoPath string) (string, error) {
	configPath := filepath.Join(repoPath, ".git", "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read git config: %w", err)
	}

	// Simple regex to extract url from [remote "origin"] section
	re := regexp.MustCompile(`\[remote "origin"\][^\[]*?url\s*=\s*(\S+)`)
	matches := re.FindStringSubmatch(string(data))
	if len(matches) < 2 {
		return "", fmt.Errorf("no origin remote found")
	}
	return matches[1], nil
}

// IsGitIgnored reports whether relPath is git-ignored in repoRoot.
// Uses "git check-ignore" so it respects nested .gitignore files.
func IsGitIgnored(repoRoot, relPath string) bool {
	cmd := exec.Command("git", "-C", repoRoot, "check-ignore", "-q", relPath)
	return cmd.Run() == nil
}

// IsTrackedInGit reports whether relPath is currently tracked by git in repoRoot.
func IsTrackedInGit(repoRoot, relPath string) bool {
	cmd := exec.Command("git", "-C", repoRoot, "ls-files", "--error-unmatch", relPath)
	return cmd.Run() == nil
}

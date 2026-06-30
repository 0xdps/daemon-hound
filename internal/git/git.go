package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Client wraps git operations for the vault repository.
type Client struct {
	vaultPath string
}

// NewClient creates a git client operating on the given vault path.
func NewClient(vaultPath string) *Client {
	return &Client{vaultPath: vaultPath}
}

// Clone clones a remote repository into the vault path.
func Clone(remoteURL, vaultPath string) error {
	if err := os.MkdirAll(filepath.Dir(vaultPath), 0755); err != nil {
		return fmt.Errorf("failed to create vault parent directory: %w", err)
	}
	cmd := exec.Command("git", "clone", remoteURL, vaultPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone failed: %w\n%s", err, string(out))
	}
	return nil
}

// CloneSparse clones a remote repository with blob:none filter and checks out
// only the specified paths. Everything else stays absent and is fetched lazily
// on demand via PullFile.
//
// Uses 'git checkout HEAD -- <path>' instead of sparse-checkout because
// sparse-checkout set + checkout HEAD with --filter=blob:none fetches the
// entire root tree object and may materialize all blobs.
func CloneSparse(remoteURL, vaultPath string, paths []string) error {
	if err := os.MkdirAll(filepath.Dir(vaultPath), 0755); err != nil {
		return fmt.Errorf("failed to create vault parent directory: %w", err)
	}

	// Clone with blob:none + no-checkout — only git metadata downloads.
	// No file content is fetched yet.
	cmd := exec.Command("git", "clone", "--filter=blob:none", "--no-checkout", remoteURL, vaultPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git sparse clone failed: %w\n%s", err, string(out))
	}

	// Always include state.toml.age so the vault index is available.
	checkoutPaths := append([]string{"state.toml.age"}, paths...)

	// Checkout exactly those paths. With --filter=blob:none, Git fetches only
	// the blobs for these specific files — nothing else materializes.
	checkoutCmd := exec.Command("git", "-C", vaultPath, "checkout", "HEAD", "--")
	checkoutCmd.Args = append(checkoutCmd.Args, checkoutPaths...)
	if out, err := checkoutCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout failed: %w\n%s", err, string(out))
	}

	return nil
}

// PullFile fetches a single file from the remote on demand.
// Uses 'git checkout HEAD -- <path>' to fetch exactly one blob from a partial
// clone, without materializing sibling files (unlike sparse-checkout add which
// fetches the entire parent tree and may materialize all blobs in that tree).
//
// The file path must be relative to the repository root.
func (c *Client) PullFile(filePath string) error {
	cmd := exec.Command("git", "-C", c.vaultPath, "checkout", "HEAD", "--", filePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s failed: %w\n%s", filePath, err, string(out))
	}
	return nil
}

// Init initializes a new git repository at the vault path.
func Init(vaultPath string) error {
	if err := os.MkdirAll(vaultPath, 0755); err != nil {
		return fmt.Errorf("failed to create vault directory: %w", err)
	}
	cmd := exec.Command("git", "-C", vaultPath, "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init failed: %w\n%s", err, string(out))
	}
	return nil
}

// AddRemote adds the origin remote if it doesn't exist.
func (c *Client) AddRemote(remoteURL string) error {
	cmd := exec.Command("git", "-C", c.vaultPath, "remote", "add", "origin", remoteURL)
	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(out), "already exists") {
		return fmt.Errorf("git remote add failed: %w\n%s", err, string(out))
	}
	return nil
}

// Pull pulls the latest changes from the remote.
func (c *Client) Pull() error {
	// --no-rebase explicitly selects merge strategy so the caller never needs
	// pull.rebase configured globally in the user's git config.
	cmd := exec.Command("git", "-C", c.vaultPath, "pull", "--no-rebase")
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Already up to date") || strings.Contains(s, "up-to-date") {
			return nil
		}
		if strings.Contains(s, "fatal: couldn't find remote ref") || strings.Contains(s, "fatal: not a git repository") {
			return nil
		}
		return fmt.Errorf("git pull failed: %w\n%s", err, s)
	}
	return nil
}

// Push pushes local commits to the remote.
func (c *Client) Push() error {
	cmd := exec.Command("git", "-C", c.vaultPath, "push", "origin", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push failed: %w\n%s", err, string(out))
	}
	return nil
}

// CommitAll stages all changes and commits with the given message.
// The current hostname is appended to every message so it's clear which
// machine originated the change, e.g.: "daemon-hound: sync [myhost]"
func (c *Client) CommitAll(message string) error {
	cmd := exec.Command("git", "-C", c.vaultPath, "add", "-A")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %w\n%s", err, string(out))
	}

	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	fullMessage := fmt.Sprintf("%s [%s]", message, host)

	cmd = exec.Command("git", "-C", c.vaultPath, "commit", "-m", fullMessage)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "nothing to commit") || strings.Contains(s, "working tree clean") {
			return nil
		}
		return fmt.Errorf("git commit failed: %w\n%s", err, s)
	}
	return nil
}

// HasChanges returns true if there are uncommitted changes.
func (c *Client) HasChanges() (bool, error) {
	cmd := exec.Command("git", "-C", c.vaultPath, "status", "--porcelain")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git status failed: %w\n%s", err, string(out))
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

// EnsureGitDir makes sure the vault path is a git repo.
func (c *Client) EnsureGitDir() error {
	gitDir := filepath.Join(c.vaultPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := Init(c.vaultPath); err != nil {
			return err
		}
	}
	return nil
}

// IsInMerge returns true when the vault repo has an in-progress merge that has
// not yet been committed or aborted (i.e. MERGE_HEAD exists).
func (c *Client) IsInMerge() bool {
	_, err := os.Stat(filepath.Join(c.vaultPath, ".git", "MERGE_HEAD"))
	return err == nil
}

// VaultPath returns the path this client operates on.
func (c *Client) VaultPath() string {
	return c.vaultPath
}

// CheckoutTheirs resolves a single conflicted file by taking the remote (theirs)
// version. Unlike ResolveConflictsRemote which touches all files, this is scoped
// to one path so successfully smart-merged files are not overwritten.
func (c *Client) CheckoutTheirs(filePath string) error {
	cmd := exec.Command("git", "-C", c.vaultPath, "checkout", "--theirs", "--", filePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout --theirs %s failed: %w\n%s", filePath, err, string(out))
	}
	return nil
}

// CheckoutOurs resolves a single conflicted file by keeping the local (ours)
// version. Scoped to one path so other files are not affected.
func (c *Client) CheckoutOurs(filePath string) error {
	cmd := exec.Command("git", "-C", c.vaultPath, "checkout", "--ours", "--", filePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout --ours %s failed: %w\n%s", filePath, err, string(out))
	}
	return nil
}

// StageOnly stages a single file (git add <path>) without touching any other files.
func (c *Client) StageOnly(filePath string) error {
	cmd := exec.Command("git", "-C", c.vaultPath, "add", "--", filePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add %s failed: %w\n%s", filePath, err, string(out))
	}
	return nil
}

// AbortMerge aborts an in-progress merge, restoring the working tree to the
// pre-merge state. Safe to call even when no merge is in progress.
func (c *Client) AbortMerge() error {
	cmd := exec.Command("git", "-C", c.vaultPath, "merge", "--abort")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git merge --abort failed: %w\n%s", err, string(out))
	}
	return nil
}

// ResolveConflictsRemote resolves all merge conflicts by taking the remote
// (theirs) version of every conflicted file, then stages them.
// Used for vault .age files where binary content makes text merging impossible
// and the remote is considered authoritative.
func (c *Client) ResolveConflictsRemote() error {
	cmd := exec.Command("git", "-C", c.vaultPath, "checkout", "--theirs", ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to checkout --theirs: %w\n%s", err, string(out))
	}
	cmd = exec.Command("git", "-C", c.vaultPath, "add", "-A")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stage resolved files: %w\n%s", err, string(out))
	}
	return nil
}

// HasConflicts checks if there are merge conflicts in the working directory.
func (c *Client) HasConflicts() (bool, error) {
	cmd := exec.Command("git", "-C", c.vaultPath, "status", "--porcelain")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git status failed: %w\n%s", err, string(out))
	}

	// Conflicted files show as "UU" or "AA" or "DD" etc. in porcelain format
	status := string(out)
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		if len(line) >= 2 {
			code := line[:2]
			// Check for conflict markers
			if (code[0] == 'U' && code[1] == 'U') ||
				(code[0] == 'A' && code[1] == 'A') ||
				(code[0] == 'D' && code[1] == 'D') {
				return true, nil
			}
		}
	}

	return false, nil
}

// ResolveConflicts resolves merge conflicts using the specified strategy.
// strategy can be "local" (keep local changes) or "remote" (take remote changes).
func (c *Client) ResolveConflicts(strategy string) error {
	switch strategy {
	case "local":
		// Keep our changes: git checkout --ours
		cmd := exec.Command("git", "-C", c.vaultPath, "checkout", "--ours", ".")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to resolve conflicts (local): %w\n%s", err, string(out))
		}

	case "remote":
		// Take their changes: git checkout --theirs
		cmd := exec.Command("git", "-C", c.vaultPath, "checkout", "--theirs", ".")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to resolve conflicts (remote): %w\n%s", err, string(out))
		}

	default:
		return fmt.Errorf("unknown conflict resolution strategy: %s", strategy)
	}

	// Stage all resolved files
	cmd := exec.Command("git", "-C", c.vaultPath, "add", "-A")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stage resolved conflicts: %w\n%s", err, string(out))
	}

	return nil
}

// GetConflictedFiles returns a list of files with merge conflicts.
func (c *Client) GetConflictedFiles() ([]string, error) {
	cmd := exec.Command("git", "-C", c.vaultPath, "status", "--porcelain")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w\n%s", err, string(out))
	}

	var conflicted []string
	status := string(out)
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		if len(line) >= 3 {
			code := line[:2]
			file := strings.TrimSpace(line[3:])

			// Check for conflict markers
			if (code[0] == 'U' && code[1] == 'U') ||
				(code[0] == 'A' && code[1] == 'A') ||
				(code[0] == 'D' && code[1] == 'D') {
				conflicted = append(conflicted, file)
			}
		}
	}

	return conflicted, nil
}

// GetConflictVersions retrieves the three git-staged versions of a conflicted file:
//   - base  (:1:) — the common ancestor (before both machines diverged)
//   - local (:2:) — our (local) version
//   - remote(:3:) — their (remote) version
//
// filename must be relative to the vault repository root (e.g. "state.toml.age").
func (c *Client) GetConflictVersions(filename string) (base, local, remote []byte, err error) {
	get := func(stage int) ([]byte, error) {
		ref := fmt.Sprintf(":%d:%s", stage, filename)
		cmd := exec.Command("git", "-C", c.vaultPath, "show", ref)
		out, e := cmd.Output()
		if e != nil {
			return nil, fmt.Errorf("git show %s: %w", ref, e)
		}
		return out, nil
	}

	base, err = get(1)
	if err != nil {
		return
	}
	local, err = get(2)
	if err != nil {
		return
	}
	remote, err = get(3)
	return
}

// StageFile writes content to a file in the vault and marks it as resolved in git.
// Used after smart merge to replace the conflicted file with the merged result.
func (c *Client) StageFile(filename string, content []byte) error {
	path := filepath.Join(c.vaultPath, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("mkdir for staged file: %w", err)
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("write staged file: %w", err)
	}
	cmd := exec.Command("git", "-C", c.vaultPath, "add", filename)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add %s: %w\n%s", filename, err, string(out))
	}
	return nil
}

// SetMergeDriver configures the git merge driver for this repository.
func (c *Client) SetMergeDriver(name, driverCmd string) error {
	cmd := exec.Command("git", "-C", c.vaultPath, "config", "merge."+name+".driver", driverCmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config merge driver failed: %w\n%s", err, string(out))
	}
	return nil
}

// SetMergeDriverName sets the human-readable name for a merge driver.
func (c *Client) SetMergeDriverName(name, displayName string) error {
	cmd := exec.Command("git", "-C", c.vaultPath, "config", "merge."+name+".name", displayName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config merge driver name failed: %w\n%s", err, string(out))
	}
	return nil
}

// GetMergeDriver returns the configured merge driver command for the given name.
// Returns empty string if not configured.
func (c *Client) GetMergeDriver(name string) (string, error) {
	cmd := exec.Command("git", "-C", c.vaultPath, "config", "merge."+name+".driver")
	out, err := cmd.Output()
	if err != nil {
		return "", nil // not configured
	}
	return strings.TrimSpace(string(out)), nil
}

// HasGitAttributes reports whether a .gitattributes file exists in the vault.
func (c *Client) HasGitAttributes() bool {
	_, err := os.Stat(filepath.Join(c.vaultPath, ".gitattributes"))
	return err == nil
}

// WriteGitAttributes creates or appends to .gitattributes in the vault.
func (c *Client) WriteGitAttributes(content string) error {
	path := filepath.Join(c.vaultPath, ".gitattributes")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open .gitattributes: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("write .gitattributes: %w", err)
	}
	return nil
}

// ReadGitAttributes returns the contents of .gitattributes, or empty if missing.
func (c *Client) ReadGitAttributes() (string, error) {
	path := filepath.Join(c.vaultPath, ".gitattributes")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read .gitattributes: %w", err)
	}
	return string(data), nil
}

// SetDiffDriverName sets the human-readable name for a diff driver.
func (c *Client) SetDiffDriverName(name, displayName string) error {
	cmd := exec.Command("git", "-C", c.vaultPath, "config", "diff."+name+".name", displayName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config diff driver name failed: %w\n%s", err, string(out))
	}
	return nil
}

// SetDiffTextconv sets the textconv command for a diff driver.
func (c *Client) SetDiffTextconv(name, command string) error {
	cmd := exec.Command("git", "-C", c.vaultPath, "config", "diff."+name+".textconv", command)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config diff textconv failed: %w\n%s", err, string(out))
	}
	return nil
}

// GetDiffDriver returns the configured diff textconv for the given name.
func (c *Client) GetDiffDriver(name string) (string, error) {
	cmd := exec.Command("git", "-C", c.vaultPath, "config", "diff."+name+".textconv")
	out, err := cmd.Output()
	if err != nil {
		return "", nil // not configured
	}
	return strings.TrimSpace(string(out)), nil
}

// SetFilterName sets the human-readable name for a filter.
func (c *Client) SetFilterName(name, displayName string) error {
	cmd := exec.Command("git", "-C", c.vaultPath, "config", "filter."+name+".name", displayName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config filter name failed: %w\n%s", err, string(out))
	}
	return nil
}

// SetFilterClean sets the clean command for a filter.
func (c *Client) SetFilterClean(name, command string) error {
	cmd := exec.Command("git", "-C", c.vaultPath, "config", "filter."+name+".clean", command)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config filter clean failed: %w\n%s", err, string(out))
	}
	return nil
}

// SetFilterSmudge sets the smudge command for a filter.
func (c *Client) SetFilterSmudge(name, command string) error {
	cmd := exec.Command("git", "-C", c.vaultPath, "config", "filter."+name+".smudge", command)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config filter smudge failed: %w\n%s", err, string(out))
	}
	return nil
}

// GetFilterClean returns the configured clean command for the given filter name.
func (c *Client) GetFilterClean(name string) (string, error) {
	cmd := exec.Command("git", "-C", c.vaultPath, "config", "filter."+name+".clean")
	out, err := cmd.Output()
	if err != nil {
		return "", nil // not configured
	}
	return strings.TrimSpace(string(out)), nil
}

// SetRerereEnabled enables or disables git rerere in the repository.
func (c *Client) SetRerereEnabled(enabled bool) error {
	val := "false"
	if enabled {
		val = "true"
	}
	cmd := exec.Command("git", "-C", c.vaultPath, "config", "rerere.enabled", val)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config rerere.enabled failed: %w\n%s", err, string(out))
	}
	return nil
}

// GetRerereEnabled returns whether rerere is enabled in the repository.
func (c *Client) GetRerereEnabled() (bool, error) {
	cmd := exec.Command("git", "-C", c.vaultPath, "config", "--bool", "rerere.enabled")
	out, err := cmd.Output()
	if err != nil {
		return false, nil // not configured = disabled
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

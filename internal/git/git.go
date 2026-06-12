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
	cmd := exec.Command("git", "-C", c.vaultPath, "pull", "origin", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore "already up to date" and "no tracking information" errors
		s := string(out)
		if strings.Contains(s, "Already up to date") || strings.Contains(s, "up-to-date") {
			return nil
		}
		// If the repo is empty or has no commits, ignore
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
func (c *Client) CommitAll(message string) error {
	cmd := exec.Command("git", "-C", c.vaultPath, "add", "-A")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %w\n%s", err, string(out))
	}

	cmd = exec.Command("git", "-C", c.vaultPath, "commit", "-m", message)
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

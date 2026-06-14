package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0xdps/daemon-hound/internal/models"
)

// newTestCfg creates a Config pointing to a temp directory.
// Tests in the same package can access unexported fields.
func newTestCfg(dir string) *Config {
	return &Config{
		path: filepath.Join(dir, "config.toml"),
		data: models.MachineConfig{Bindings: make(map[string]string)},
	}
}

func TestConfigInitAndLoad(t *testing.T) {
	dir := t.TempDir()
	c := newTestCfg(dir)

	if err := c.Init("git@github.com:test/vault.git"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if c.MachineID() == "" {
		t.Error("MachineID should not be empty after Init")
	}
	if c.VaultRemote() != "git@github.com:test/vault.git" {
		t.Errorf("VaultRemote = %q, want git@github.com:test/vault.git", c.VaultRemote())
	}

	c2 := newTestCfg(dir)
	if err := c2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if c2.MachineID() != c.MachineID() {
		t.Errorf("Loaded MachineID %q != saved %q", c2.MachineID(), c.MachineID())
	}
	if c2.VaultRemote() != "git@github.com:test/vault.git" {
		t.Errorf("Loaded VaultRemote = %q", c2.VaultRemote())
	}
}

func TestConfigBindings(t *testing.T) {
	dir := t.TempDir()
	c := newTestCfg(dir)
	if err := c.Init(""); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if err := c.SetBinding("github.com/test/repo", "/tmp/repo"); err != nil {
		t.Fatalf("SetBinding failed: %v", err)
	}
	got, ok := c.GetBinding("github.com/test/repo")
	if !ok {
		t.Fatal("GetBinding returned not found")
	}
	if got != "/tmp/repo" {
		t.Errorf("GetBinding = %q, want /tmp/repo", got)
	}
	_, ok = c.GetBinding("github.com/nonexistent")
	if ok {
		t.Error("GetBinding should return false for unknown namespace")
	}
}

func TestConfigPendingPush(t *testing.T) {
	dir := t.TempDir()
	c := newTestCfg(dir)
	if err := c.Init(""); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if c.PendingPush() {
		t.Error("PendingPush should be false initially")
	}
	if err := c.SetPendingPush(true); err != nil {
		t.Fatalf("SetPendingPush(true) failed: %v", err)
	}
	if !c.PendingPush() {
		t.Error("PendingPush should be true after SetPendingPush(true)")
	}

	c2 := newTestCfg(dir)
	if err := c2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !c2.PendingPush() {
		t.Error("PendingPush should persist across Load")
	}

	if err := c.SetPendingPush(false); err != nil {
		t.Fatalf("SetPendingPush(false) failed: %v", err)
	}
	if c.PendingPush() {
		t.Error("PendingPush should be false after SetPendingPush(false)")
	}
}

func TestConfigLoadNotFound(t *testing.T) {
	c := newTestCfg(t.TempDir())
	if err := c.Load(); err == nil {
		t.Error("Load should return error when file does not exist")
	}
}

func TestConfigSavePersists(t *testing.T) {
	dir := t.TempDir()
	c := newTestCfg(dir)
	if err := c.Init("git@test.example.com:vault.git"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	cfgPath := filepath.Join(dir, "config.toml")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("Config file not created: %v", err)
	}
}

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

func TestConfigIgnoredFiles(t *testing.T) {
	dir := t.TempDir()
	c := newTestCfg(dir)
	if err := c.Init(""); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if c.IsFileIgnored("github.com/test/repo", ".env.local") {
		t.Fatal("file should not be ignored initially")
	}
	if err := c.IgnoreFile("github.com/test/repo", ".env.local"); err != nil {
		t.Fatalf("IgnoreFile failed: %v", err)
	}
	if err := c.IgnoreFile("github.com/test/repo", ".env.local"); err != nil {
		t.Fatalf("duplicate IgnoreFile failed: %v", err)
	}
	if !c.IsFileIgnored("github.com/test/repo", ".env.local") {
		t.Fatal("file should be ignored")
	}
	if got := c.IgnoredFiles(); len(got) != 1 || got[0] != "github.com/test/repo:.env.local" {
		t.Fatalf("IgnoredFiles = %#v", got)
	}

	c2 := newTestCfg(dir)
	if err := c2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !c2.IsFileIgnored("github.com/test/repo", ".env.local") {
		t.Fatal("ignored file should persist across Load")
	}

	if err := c2.UnignoreFile("github.com/test/repo", ".env.local"); err != nil {
		t.Fatalf("UnignoreFile failed: %v", err)
	}
	if c2.IsFileIgnored("github.com/test/repo", ".env.local") {
		t.Fatal("file should not be ignored after UnignoreFile")
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

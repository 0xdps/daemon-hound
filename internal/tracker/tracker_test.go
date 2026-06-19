package tracker

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/0xdps/daemon-hound/internal/storage"
	"github.com/0xdps/daemon-hound/internal/utils"
)

type mockConfig struct {
	machineID string
	bindings  map[string]string
}

func (m *mockConfig) MachineID() string                   { return m.machineID }
func (m *mockConfig) GetBinding(ns string) (string, bool) { v, ok := m.bindings[ns]; return v, ok }
func (m *mockConfig) SetBinding(ns, path string) error    { m.bindings[ns] = path; return nil }
func (m *mockConfig) UnignoreFile(ns, path string) error  { return nil }

// calculateChecksum computes a simple checksum for testing
func calculateChecksum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func TestTrackerStatus(t *testing.T) {
	tmpDir := t.TempDir()
	identity, err := storage.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	vault := storage.NewVault(tmpDir, identity)
	cfg := &mockConfig{
		machineID: "test-machine",
		bindings:  map[string]string{"github.com/test/repo": tmpDir},
	}
	tr := NewTracker(vault, cfg)

	// Create a test file
	testFile := filepath.Join(tmpDir, ".env.local")
	content := []byte("KEY=value")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	checksum, err := calculateChecksum(testFile)
	if err != nil {
		t.Fatal(err)
	}

	file := models.TrackedFile{
		Namespace: "github.com/test/repo",
		RelPath:   ".env.local",
		Mode:      models.ModeSync,
		Checksum:  checksum,
	}

	// Status should be clean
	status, err := tr.Status(file)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status != models.StatusClean {
		t.Errorf("Expected status clean, got %s", status)
	}

	// Modify file
	if err := os.WriteFile(testFile, []byte("KEY=newvalue"), 0644); err != nil {
		t.Fatal(err)
	}

	status, err = tr.Status(file)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status != models.StatusDirty {
		t.Errorf("Expected status dirty, got %s", status)
	}

	// Delete file
	os.Remove(testFile)
	status, err = tr.Status(file)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status != models.StatusMissing {
		t.Errorf("Expected status missing, got %s", status)
	}
}

func TestTrackerRestore(t *testing.T) {
	tmpDir := t.TempDir()
	identity, err := storage.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	vault := storage.NewVault(tmpDir, identity)
	cfg := &mockConfig{
		machineID: "test-machine",
		bindings:  map[string]string{"github.com/test/repo": tmpDir},
	}
	tr := NewTracker(vault, cfg)

	// Store a file in the vault
	file := models.TrackedFile{
		Namespace: "github.com/test/repo",
		RelPath:   ".env.local",
		Mode:      models.ModeSync,
	}
	plaintext := []byte("RESTORED=value")
	if _, err := vault.StoreFile(file, plaintext); err != nil {
		t.Fatalf("StoreFile failed: %v", err)
	}

	// Restore it
	restoredPath, err := tr.Restore(file)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// Verify content
	content, err := os.ReadFile(restoredPath)
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}
	if string(content) != string(plaintext) {
		t.Errorf("Restored content mismatch: got %q, want %q", content, plaintext)
	}
}

func TestTrackerRestoreGlobal(t *testing.T) {
	tmpDir := t.TempDir()
	identity, err := storage.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	vault := storage.NewVault(tmpDir, identity)
	cfg := &mockConfig{
		machineID: "test-machine",
		bindings:  map[string]string{},
	}
	tr := NewTracker(vault, cfg)

	// Store a global file
	file := models.TrackedFile{
		Namespace: "global",
		RelPath:   ".zshrc",
		Mode:      models.ModeBackup,
		MachineID: "test-machine",
	}
	plaintext := []byte("alias ll='ls -la'")
	if _, err := vault.StoreFile(file, plaintext); err != nil {
		t.Fatalf("StoreFile failed: %v", err)
	}

	// Restore it
	restoredPath, err := tr.Restore(file)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// Verify content
	content, err := os.ReadFile(restoredPath)
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}
	if string(content) != string(plaintext) {
		t.Errorf("Restored content mismatch: got %q, want %q", content, plaintext)
	}
}

func TestTrackerStatusFallsBackToRepoDiscovery(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	repoRoot := filepath.Join(tmpHome, "personal", "0xdps", "pinboard-gpt-extension")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".git", "config"), []byte(`[remote "origin"]
	url = https://github.com/0xdps/pinboard-gpt-extension.git
`), 0644); err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(repoRoot, ".env.local")
	if err := os.WriteFile(filePath, []byte("KEY=value"), 0644); err != nil {
		t.Fatal(err)
	}

	checksum, err := calculateChecksum(filePath)
	if err != nil {
		t.Fatal(err)
	}

	identity, err := storage.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	vault := storage.NewVault(t.TempDir(), identity)
	cfg := &mockConfig{machineID: "test-machine", bindings: map[string]string{}}
	tr := NewTracker(vault, cfg)

	namespace, err := utils.DeriveNamespace("https://github.com/0xdps/pinboard-gpt-extension.git")
	if err != nil {
		t.Fatal(err)
	}

	status, err := tr.Status(models.TrackedFile{
		Namespace: namespace,
		RelPath:   ".env.local",
		Mode:      models.ModeSync,
		Checksum:  checksum,
	})
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status != models.StatusClean {
		t.Fatalf("expected status clean, got %s", status)
	}
}

func TestTrackerStatusNew(t *testing.T) {
	tmpDir := t.TempDir()
	identity, _ := storage.GenerateIdentity()
	vault := storage.NewVault(tmpDir, identity)
	cfg := &mockConfig{
		machineID: "test-machine",
		bindings:  map[string]string{"github.com/test/repo": tmpDir},
	}
	tr := NewTracker(vault, cfg)

	testFile := filepath.Join(tmpDir, ".env.new")
	if err := os.WriteFile(testFile, []byte("NEW=1"), 0644); err != nil {
		t.Fatal(err)
	}
	file := models.TrackedFile{
		Namespace: "github.com/test/repo",
		RelPath:   ".env.new",
		Mode:      models.ModeSync,
		Checksum:  "",
	}
	status, err := tr.Status(file)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status != models.StatusNew {
		t.Errorf("Expected StatusNew for empty checksum, got %s", status)
	}
}

func TestTrackerResolveKeyGlobal(t *testing.T) {
	tmpDir := t.TempDir()
	identity, _ := storage.GenerateIdentity()
	vault := storage.NewVault(tmpDir, identity)
	cfg := &mockConfig{machineID: "m", bindings: map[string]string{}}
	tr := NewTracker(vault, cfg)

	home, _ := os.UserHomeDir()
	testFile := filepath.Join(home, ".zshrc")

	ns, rel, err := tr.ResolveKey(testFile)
	if err != nil {
		t.Fatalf("ResolveKey failed: %v", err)
	}
	if ns != "global" {
		t.Errorf("namespace = %q, want global", ns)
	}
	if rel != ".zshrc" {
		t.Errorf("relPath = %q, want .zshrc", rel)
	}
}

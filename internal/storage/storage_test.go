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

package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0xdps/daemon-hound/internal/models"
)

func TestVaultStoreAndRetrieve(t *testing.T) {
	tmpDir := t.TempDir()
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	vault := NewVault(tmpDir, identity)
	plaintext := []byte("hello, secret world!")

	file := models.TrackedFile{
		Namespace: "github.com/test/repo",
		RelPath:   ".env.local",
		Mode:      models.ModeSync,
	}

	// Store
	if _, err := vault.StoreFile(file, plaintext); err != nil {
		t.Fatalf("StoreFile failed: %v", err)
	}

	// Verify file exists
	vaultFilePath := filepath.Join(tmpDir, "sync", "github.com/test/repo", ".env.local.age")
	if _, err := os.Stat(vaultFilePath); err != nil {
		t.Fatalf("Vault file not created: %v", err)
	}

	// Retrieve
	retrieved, err := vault.RetrieveFile(file)
	if err != nil {
		t.Fatalf("RetrieveFile failed: %v", err)
	}
	if string(retrieved) != string(plaintext) {
		t.Errorf("Retrieved content mismatch: got %q, want %q", retrieved, plaintext)
	}
}

func TestVaultFileExists(t *testing.T) {
	tmpDir := t.TempDir()
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	vault := NewVault(tmpDir, identity)
	file := models.TrackedFile{
		Namespace: "github.com/test/repo",
		RelPath:   ".env.local",
		Mode:      models.ModeSync,
	}

	if vault.FileExistsInVault(file) {
		t.Error("FileExistsInVault should be false for non-existent file")
	}

	if _, err := vault.StoreFile(file, []byte("test")); err != nil {
		t.Fatalf("StoreFile failed: %v", err)
	}

	if !vault.FileExistsInVault(file) {
		t.Error("FileExistsInVault should be true after storing")
	}
}

func TestVaultRemoveFile(t *testing.T) {
	tmpDir := t.TempDir()
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	vault := NewVault(tmpDir, identity)
	file := models.TrackedFile{
		Namespace: "github.com/test/repo",
		RelPath:   ".env.local",
		Mode:      models.ModeSync,
	}

	if _, err := vault.StoreFile(file, []byte("test")); err != nil {
		t.Fatalf("StoreFile failed: %v", err)
	}

	if err := vault.RemoveFile(file); err != nil {
		t.Fatalf("RemoveFile failed: %v", err)
	}

	if vault.FileExistsInVault(file) {
		t.Error("File should not exist after removal")
	}
}

func TestVaultState(t *testing.T) {
	tmpDir := t.TempDir()
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	vault := NewVault(tmpDir, identity)

	// Load fresh state
	state, err := vault.LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if state.Version != "1" {
		t.Errorf("Expected version '1', got %q", state.Version)
	}
	if state.Files == nil {
		t.Error("Files map should be initialized")
	}
	if state.Secrets == nil {
		t.Error("Secrets map should be initialized")
	}

	// Add a file and save
	state.Files["github.com/test/repo:.env.local"] = models.TrackedFile{
		Namespace: "github.com/test/repo",
		RelPath:   ".env.local",
		Mode:      models.ModeSync,
	}
	if err := vault.SaveState(state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Reload and verify
	state2, err := vault.LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	file, ok := state2.Files["github.com/test/repo:.env.local"]
	if !ok {
		t.Fatal("File not found in reloaded state")
	}
	if file.Namespace != "github.com/test/repo" {
		t.Errorf("Wrong namespace: %q", file.Namespace)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	vault := NewVault("", identity)
	plaintext := []byte("sensitive data here")

	ciphertext, err := vault.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if len(ciphertext) == 0 {
		t.Error("Encrypt returned empty ciphertext")
	}

	decrypted, err := vault.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestParseIdentity(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	identityStr := identity.String()
	parsed, err := ParseIdentity(identityStr)
	if err != nil {
		t.Fatalf("ParseIdentity failed: %v", err)
	}

	if parsed.String() != identityStr {
		t.Error("Parsed identity does not match original")
	}
}

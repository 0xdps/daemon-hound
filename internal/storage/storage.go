package storage

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"filippo.io/age"
	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/BurntSushi/toml"
)

// Vault manages the on-disk vault: encryption, layout, and state.
type Vault struct {
	path     string
	identity *age.X25519Identity
}

// NewVault opens the vault at the given path. The identity is used for encryption/decryption.
func NewVault(vaultPath string, identity *age.X25519Identity) *Vault {
	return &Vault{path: vaultPath, identity: identity}
}

// GenerateIdentity creates a new age identity.
func GenerateIdentity() (*age.X25519Identity, error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("failed to generate age identity: %w", err)
	}
	return identity, nil
}

// ParseIdentity parses an age identity from a string.
func ParseIdentity(s string) (*age.X25519Identity, error) {
	identity, err := age.ParseX25519Identity(s)
	if err != nil {
		return nil, fmt.Errorf("failed to parse age identity: %w", err)
	}
	return identity, nil
}

// Recipient returns the public recipient for this vault's identity.
func (v *Vault) Recipient() age.Recipient {
	return v.identity.Recipient()
}

// Encrypt encrypts plaintext using the vault's identity recipient.
func (v *Vault) Encrypt(plaintext []byte) ([]byte, error) {
	var out bytes.Buffer
	w, err := age.Encrypt(&out, v.identity.Recipient())
	if err != nil {
		return nil, fmt.Errorf("failed to create age encryptor: %w", err)
	}
	if _, err := w.Write(plaintext); err != nil {
		return nil, fmt.Errorf("failed to encrypt data: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close age encryptor: %w", err)
	}
	return out.Bytes(), nil
}

// Decrypt decrypts ciphertext using the vault's identity.
func (v *Vault) Decrypt(ciphertext []byte) ([]byte, error) {
	r, err := age.Decrypt(bytes.NewReader(ciphertext), v.identity)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}
	var out bytes.Buffer
	if _, err := out.ReadFrom(r); err != nil {
		return nil, fmt.Errorf("failed to read decrypted data: %w", err)
	}
	return out.Bytes(), nil
}

// VaultStatePath returns the path to the encrypted vault state file.
func (v *Vault) VaultStatePath() string {
	return filepath.Join(v.path, "state.toml.age")
}

// legacyStatePath returns the old unencrypted state path (for migration).
func (v *Vault) legacyStatePath() string {
	return filepath.Join(v.path, "state.toml")
}

// LoadState reads and decrypts the vault state from disk.
// Falls back to the legacy unencrypted state.toml for seamless migration.
// Returns a fresh state if neither file is found.
func (v *Vault) LoadState() (*models.VaultState, error) {
	state := &models.VaultState{
		Version: "1",
		Files:   make(map[string]models.TrackedFile),
		Secrets: make(map[string]models.Secret),
	}

	var data []byte

	if enc, err := os.ReadFile(v.VaultStatePath()); err == nil {
		// Encrypted state file found — decrypt it.
		plain, decErr := v.Decrypt(enc)
		if decErr != nil {
			return nil, fmt.Errorf("failed to decrypt vault state: %w", decErr)
		}
		data = plain
	} else if plain, legacyErr := os.ReadFile(v.legacyStatePath()); legacyErr == nil {
		// Legacy unencrypted state.toml — will be migrated on next SaveState.
		data = plain
	} else {
		// No state file yet — return empty state.
		return state, nil
	}

	if _, err := toml.Decode(string(data), state); err != nil {
		return nil, fmt.Errorf("failed to decode vault state: %w", err)
	}
	if state.Files == nil {
		state.Files = make(map[string]models.TrackedFile)
	}
	if state.Secrets == nil {
		state.Secrets = make(map[string]models.Secret)
	}
	return state, nil
}

// SaveState encrypts and writes the vault state to disk only when the content
// has changed. Because age uses a random nonce on every Encrypt call the
// ciphertext differs even for identical plaintext, so we compare a SHA-256 of
// the TOML encoding against the decrypted on-disk content before writing.
// Any legacy plain state.toml is removed after a successful write.
func (v *Vault) SaveState(state *models.VaultState) error {
	if err := os.MkdirAll(v.path, 0755); err != nil {
		return fmt.Errorf("failed to create vault directory: %w", err)
	}

	// Encode to TOML in memory.
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(state); err != nil {
		return fmt.Errorf("failed to encode vault state: %w", err)
	}
	newToml := buf.Bytes()

	// Skip the write if the on-disk plaintext is identical — this avoids a
	// spurious age re-encryption (different nonce → different ciphertext →
	// unnecessary git commit on every sync cycle).
	if existing, err := os.ReadFile(v.VaultStatePath()); err == nil {
		if plain, decErr := v.Decrypt(existing); decErr == nil {
			if sha256Equal(plain, newToml) {
				return nil
			}
		}
	}

	// Encrypt the TOML bytes.
	enc, err := v.Encrypt(newToml)
	if err != nil {
		return fmt.Errorf("failed to encrypt vault state: %w", err)
	}

	// Write atomically via a temp file.
	tmp := v.VaultStatePath() + ".tmp"
	if err := os.WriteFile(tmp, enc, 0600); err != nil {
		return fmt.Errorf("failed to write vault state: %w", err)
	}
	if err := os.Rename(tmp, v.VaultStatePath()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to commit vault state: %w", err)
	}

	// Remove legacy plain state.toml if it still exists.
	_ = os.Remove(v.legacyStatePath())
	return nil
}

// StoreFile encrypts and writes a file into the vault layout.
// For sync mode: vault/sync/<namespace>/<relPath>.age
// For backup mode: vault/backup/<machineID>/<namespace>/<relPath>.age
// Returns (false, nil) when the on-disk plaintext is already identical (no write performed),
// (true, nil) when the file was written, or (false, err) on failure.
// Skipping the write avoids spurious age re-encryptions (different nonce → different
// ciphertext → empty git commits on every sync cycle).
func (v *Vault) StoreFile(file models.TrackedFile, plaintext []byte) (bool, error) {
	var dir string
	if file.Mode == models.ModeBackup {
		dir = filepath.Join(v.path, "backup", file.MachineID, file.Namespace)
	} else {
		dir = filepath.Join(v.path, "sync", file.Namespace)
	}
	vaultFilePath := filepath.Join(dir, file.RelPath+".age")

	// Skip write if on-disk plaintext is identical.
	if existing, err := os.ReadFile(vaultFilePath); err == nil {
		if current, decErr := v.Decrypt(existing); decErr == nil && sha256Equal(current, plaintext) {
			return false, nil
		}
	}

	enc, err := v.Encrypt(plaintext)
	if err != nil {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(vaultFilePath), 0755); err != nil {
		return false, fmt.Errorf("failed to create vault file directory: %w", err)
	}
	if err := os.WriteFile(vaultFilePath, enc, 0644); err != nil {
		return false, fmt.Errorf("failed to write vault file: %w", err)
	}
	return true, nil
}

// RetrieveFile reads and decrypts a file from the vault layout.
func (v *Vault) RetrieveFile(file models.TrackedFile) ([]byte, error) {
	var vaultFilePath string
	if file.Mode == models.ModeBackup {
		vaultFilePath = filepath.Join(v.path, "backup", file.MachineID, file.Namespace, file.RelPath+".age")
	} else {
		vaultFilePath = filepath.Join(v.path, "sync", file.Namespace, file.RelPath+".age")
	}

	enc, err := os.ReadFile(vaultFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read vault file: %w", err)
	}
	return v.Decrypt(enc)
}

// FileExistsInVault checks if the encrypted file exists in the vault.
func (v *Vault) FileExistsInVault(file models.TrackedFile) bool {
	var vaultFilePath string
	if file.Mode == models.ModeBackup {
		vaultFilePath = filepath.Join(v.path, "backup", file.MachineID, file.Namespace, file.RelPath+".age")
	} else {
		vaultFilePath = filepath.Join(v.path, "sync", file.Namespace, file.RelPath+".age")
	}
	_, err := os.Stat(vaultFilePath)
	return err == nil
}

// RemoveFile removes a file from the vault layout.
func (v *Vault) RemoveFile(file models.TrackedFile) error {
	var vaultFilePath string
	if file.Mode == models.ModeBackup {
		vaultFilePath = filepath.Join(v.path, "backup", file.MachineID, file.Namespace, file.RelPath+".age")
	} else {
		vaultFilePath = filepath.Join(v.path, "sync", file.Namespace, file.RelPath+".age")
	}
	if err := os.Remove(vaultFilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove vault file: %w", err)
	}
	return nil
}

// sha256Equal reports whether two byte slices have the same SHA-256 digest.
func sha256Equal(a, b []byte) bool {
	da := sha256.Sum256(a)
	db := sha256.Sum256(b)
	return da == db
}

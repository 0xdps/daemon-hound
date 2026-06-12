package storage

import (
	"bytes"
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

// VaultStatePath returns the path to the vault state file.
func (v *Vault) VaultStatePath() string {
	return filepath.Join(v.path, "state.toml")
}

// LoadState reads the vault state from disk. Returns a fresh state if not found.
func (v *Vault) LoadState() (*models.VaultState, error) {
	state := &models.VaultState{
		Version: "1",
		Files:   make(map[string]models.TrackedFile),
		Secrets: make(map[string]models.Secret),
	}
	path := v.VaultStatePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return state, nil
	}
	if _, err := toml.DecodeFile(path, state); err != nil {
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

// SaveState writes the vault state to disk.
func (v *Vault) SaveState(state *models.VaultState) error {
	if err := os.MkdirAll(v.path, 0755); err != nil {
		return fmt.Errorf("failed to create vault directory: %w", err)
	}
	f, err := os.OpenFile(v.VaultStatePath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open vault state file: %w", err)
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(state); err != nil {
		return fmt.Errorf("failed to encode vault state: %w", err)
	}
	return nil
}

// StoreFile encrypts and writes a file into the vault layout.
// For sync mode: vault/sync/<namespace>/<relPath>.age
// For backup mode: vault/backup/<machineID>/<namespace>/<relPath>.age
func (v *Vault) StoreFile(file models.TrackedFile, plaintext []byte) error {
	enc, err := v.Encrypt(plaintext)
	if err != nil {
		return err
	}

	var dir string
	if file.Mode == models.ModeBackup {
		dir = filepath.Join(v.path, "backup", file.MachineID, file.Namespace)
	} else {
		dir = filepath.Join(v.path, "sync", file.Namespace)
	}

	vaultFilePath := filepath.Join(dir, file.RelPath+".age")
	if err := os.MkdirAll(filepath.Dir(vaultFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create vault file directory: %w", err)
	}
	if err := os.WriteFile(vaultFilePath, enc, 0644); err != nil {
		return fmt.Errorf("failed to write vault file: %w", err)
	}
	return nil
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

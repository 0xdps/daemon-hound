package storage

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// Path returns the vault root directory.
func (v *Vault) Path() string {
	return v.path
}

// SecretsDir returns the path to the secrets subdirectory.
func (v *Vault) SecretsDir() string {
	return filepath.Join(v.path, "secrets")
}

// SecretFilePath returns the path to a named secret's encrypted TOML file.
func (v *Vault) SecretFilePath(name string) string {
	return filepath.Join(v.SecretsDir(), name+".toml.age")
}

// legacyStatePath returns the old unencrypted state path (for migration).
func (v *Vault) legacyStatePath() string {
	return filepath.Join(v.path, "state.toml")
}

// LoadState reads and decrypts the vault state from disk.
// Falls back to the legacy unencrypted state.toml for seamless reading.
// Returns a fresh state if neither file is found.
//
// LoadState does NOT migrate legacy secrets. Call MigrateIfNeeded first
// when running in a context that can commit and push (i.e. the CLI commands).
func (v *Vault) LoadState() (*models.VaultState, error) {
	state := &models.VaultState{
		Version: "1",
		Files:   make(map[string]models.TrackedFile),
		Secrets: make(map[string]models.SecretIndex),
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
		// Legacy unencrypted state.toml — readable as-is.
		data = plain
	} else {
		// No state file yet — return empty state.
		return state, nil
	}

	// Decode as new format. Legacy format fields (value, refs inline) are simply
	// ignored by the TOML decoder when missing from VaultState — the Secrets map
	// will have SecretIndex entries with only the index fields populated.
	if _, err := toml.Decode(string(data), state); err != nil {
		return nil, fmt.Errorf("failed to decode vault state: %w", err)
	}
	if state.Files == nil {
		state.Files = make(map[string]models.TrackedFile)
	}
	if state.Secrets == nil {
		state.Secrets = make(map[string]models.SecretIndex)
	}
	return state, nil
}

// HasLegacySecrets reports whether the on-disk vault state contains inline
// secrets (legacy format). Returns false if the state cannot be read.
func (v *Vault) HasLegacySecrets() bool {
	data, err := v.rawStateBytes()
	if err != nil {
		return false
	}
	var legacy models.LegacyVaultState
	legacy.Secrets = make(map[string]models.LegacySecret)
	if _, err := toml.Decode(string(data), &legacy); err != nil {
		return false
	}
	for _, s := range legacy.Secrets {
		if len(s.Value) > 0 {
			return true
		}
	}
	return false
}

// rawStateBytes decrypts and returns the raw TOML bytes of the state file.
func (v *Vault) rawStateBytes() ([]byte, error) {
	if enc, err := os.ReadFile(v.VaultStatePath()); err == nil {
		plain, decErr := v.Decrypt(enc)
		if decErr != nil {
			return nil, decErr
		}
		return plain, nil
	}
	if plain, err := os.ReadFile(v.legacyStatePath()); err == nil {
		return plain, nil
	}
	return nil, fmt.Errorf("no state file found")
}

// MigrateIfNeeded checks whether the vault state is in legacy format and, if so,
// migrates all inline secrets to per-secret files and rewrites state.toml.age.
//
// Migration is atomic in the sense that:
//   - Per-secret files are written before state.toml.age is updated.
//   - If a per-secret file already exists (from a previous partial migration),
//     it is not overwritten — the existing file is kept as-is.
//   - state.toml.age is only updated after ALL per-secret files are written.
//
// Returns (true, nil) if migration was performed, (false, nil) if not needed.
func (v *Vault) MigrateIfNeeded() (bool, error) {
	data, err := v.rawStateBytes()
	if err != nil {
		return false, nil // no state to migrate
	}

	var legacy models.LegacyVaultState
	legacy.Files = make(map[string]models.TrackedFile)
	legacy.Secrets = make(map[string]models.LegacySecret)
	if _, err := toml.Decode(string(data), &legacy); err != nil {
		return false, fmt.Errorf("decode state for migration: %w", err)
	}

	// Check if any secret has an inline value.
	hasLegacy := false
	for _, s := range legacy.Secrets {
		if len(s.Value) > 0 {
			hasLegacy = true
			break
		}
	}
	if !hasLegacy {
		return false, nil
	}

	// Write per-secret files. Skip any that already exist (idempotent).
	for name, secret := range legacy.Secrets {
		if len(secret.Value) == 0 {
			continue // no inline value — already migrated or empty
		}
		path := v.SecretFilePath(name)
		if _, err := os.Stat(path); err == nil {
			continue // already exists — skip to avoid overwriting
		}
		sf := &models.SecretFile{
			Name:      name,
			CreatedBy: "migrated",
			CreatedAt: secret.UpdatedAt,
			Latest:    "v1",
			Versions: map[string]models.SecretVersion{
				"v1": {
					CreatedAt: secret.UpdatedAt,
					Reason:    "Migrated from legacy storage",
					Value:     secret.Value, // already inner-encrypted
				},
			},
			Refs: secret.Refs,
		}
		if err := v.SaveSecretFile(sf); err != nil {
			return false, fmt.Errorf("migrate secret %s: %w", name, err)
		}
	}

	// All per-secret files written — now rewrite state.toml.age without inline secrets.
	newState := &models.VaultState{
		Version: legacy.Version,
		Files:   legacy.Files,
		Secrets: make(map[string]models.SecretIndex),
	}
	if newState.Files == nil {
		newState.Files = make(map[string]models.TrackedFile)
	}
	for name, secret := range legacy.Secrets {
		newState.Secrets[name] = models.SecretIndex{
			Latest:    "v1",
			Versions:  1,
			UpdatedAt: secret.UpdatedAt,
		}
	}
	if err := v.SaveState(newState); err != nil {
		return false, fmt.Errorf("save state after migration: %w", err)
	}

	return true, nil
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

// LoadSecretFile reads and decrypts a per-secret TOML file from disk.
func (v *Vault) LoadSecretFile(name string) (*models.SecretFile, error) {
	path := v.SecretFilePath(name)
	enc, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("secret not found: %s", name)
		}
		return nil, fmt.Errorf("failed to read secret file: %w", err)
	}

	plain, err := v.Decrypt(enc)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret file: %w", err)
	}

	sf := &models.SecretFile{}
	if _, err := toml.Decode(string(plain), sf); err != nil {
		return nil, fmt.Errorf("failed to decode secret file: %w", err)
	}
	if sf.Versions == nil {
		sf.Versions = make(map[string]models.SecretVersion)
	}
	return sf, nil
}

// SaveSecretFile encrypts and writes a per-secret TOML file to disk.
func (v *Vault) SaveSecretFile(sf *models.SecretFile) error {
	if err := os.MkdirAll(v.SecretsDir(), 0755); err != nil {
		return fmt.Errorf("failed to create secrets directory: %w", err)
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(sf); err != nil {
		return fmt.Errorf("failed to encode secret file: %w", err)
	}

	enc, err := v.Encrypt(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to encrypt secret file: %w", err)
	}

	path := v.SecretFilePath(sf.Name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, enc, 0600); err != nil {
		return fmt.Errorf("failed to write secret file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to commit secret file: %w", err)
	}
	return nil
}

// DeleteSecretFile removes a per-secret TOML file from disk.
func (v *Vault) DeleteSecretFile(name string) error {
	path := v.SecretFilePath(name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete secret file: %w", err)
	}
	return nil
}

// UpdateSecretIndex rebuilds the lightweight secret index in VaultState
// from the actual per-secret files on disk. Call this after any secret
// mutation (set, rotate, rollback, delete, rename) so the index stays
// in sync with the files.
func (v *Vault) UpdateSecretIndex(state *models.VaultState) error {
	state.Secrets = make(map[string]models.SecretIndex)
	names, err := v.ListSecretNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		sf, err := v.LoadSecretFile(name)
		if err != nil {
			continue // skip unreadable files
		}
		updatedAt := sf.CreatedAt
		if ver, ok := sf.Versions[sf.Latest]; ok {
			updatedAt = ver.CreatedAt
		}
		state.Secrets[name] = models.SecretIndex{
			Latest:    sf.Latest,
			Versions:  len(sf.Versions),
			UpdatedAt: updatedAt,
		}
	}
	return nil
}

// ReadLegacySecret decrypts and returns the value of a secret stored inline in
// state.toml.age (legacy format). Returns (nil, nil) if the secret is not found
// in legacy format, or an error if decryption fails.
func (v *Vault) ReadLegacySecret(name string) ([]byte, error) {
	enc, err := os.ReadFile(v.VaultStatePath())
	if err != nil {
		return nil, fmt.Errorf("read vault state: %w", err)
	}
	plain, err := v.Decrypt(enc)
	if err != nil {
		return nil, fmt.Errorf("decrypt vault state: %w", err)
	}

	// Decode as legacy format to get inline secret values.
	var legacyState models.LegacyVaultState
	legacyState.Secrets = make(map[string]models.LegacySecret)
	if _, err := toml.Decode(string(plain), &legacyState); err != nil {
		return nil, fmt.Errorf("decode vault state: %w", err)
	}
	s, ok := legacyState.Secrets[name]
	if !ok || len(s.Value) == 0 {
		return nil, nil // not in legacy format
	}
	// The value is age-encrypted; decrypt it.
	return v.Decrypt(s.Value)
}

// ListSecretNames returns all secret names by listing the secrets directory.
func (v *Vault) ListSecretNames() ([]string, error) {
	entries, err := os.ReadDir(v.SecretsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".toml.age") {
			names = append(names, strings.TrimSuffix(name, ".toml.age"))
		}
	}
	return names, nil
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

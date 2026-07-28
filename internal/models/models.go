package models

import "time"

// FileMode indicates whether a tracked file is synced across machines or backup-only.
type FileMode string

const (
	ModeSync   FileMode = "sync"
	ModeBackup FileMode = "backup"
)

// DaemonConfig holds daemon-specific settings.
// Most daemon behaviour uses hardcoded defaults (30s poll, 10MB/5MB log rotation,
// 30-day retention, smart-merge conflict strategy). These are reserved for
// future configuration in v1.2.0+.
type DaemonConfig struct {
	PollInterval int `toml:"poll_interval"` // reserved: remote poll interval in seconds (default 30)
}

// MachineConfig holds per-machine DaemonHound configuration.
type MachineConfig struct {
	MachineID    string            `toml:"machine_id"`
	VaultRemote  string            `toml:"vault_remote,omitempty"`
	IdentitySalt string            `toml:"identity_salt,omitempty"` // prefix@postfix salt for identity encryption (global across machines)
	Bindings     map[string]string `toml:"bindings"`                // namespace -> local absolute path
	IgnoredFiles []string          `toml:"ignored_files,omitempty"` // local-only ignores, key: "namespace:relPath"
	PendingPush  bool              `toml:"pending_push,omitempty"`  // local vault commits not yet pushed to remote
	Daemon       DaemonConfig      `toml:"daemon"`                  // daemon configuration
}

// TrackedFile represents a single file tracked by DaemonHound.
type TrackedFile struct {
	Namespace  string    `toml:"namespace"`            // e.g. github.com/dps/pingpong-api or "global"
	RelPath    string    `toml:"rel_path"`             // path relative to namespace root
	Mode       FileMode  `toml:"mode"`                 // sync or backup
	MachineID  string    `toml:"machine_id,omitempty"` // set for backup mode
	LastSyncAt time.Time `toml:"last_sync_at"`
	Checksum   string    `toml:"checksum"` // SHA-256 of plaintext content at last sync
}

// VaultState is the top-level state file stored in the vault repo.
type VaultState struct {
	Version string                 `toml:"version"`
	Files   map[string]TrackedFile `toml:"files"`   // key: "namespace:relPath"
	Secrets map[string]SecretIndex `toml:"secrets"` // key: secret name — lightweight index
}

// SecretIndex is a lightweight pointer to a per-secret file.
// The actual values live in vault/secrets/<name>.toml.age.
type SecretIndex struct {
	Latest    string    `toml:"latest"`
	Versions  int       `toml:"versions"`
	UpdatedAt time.Time `toml:"updated_at"`
}

// SecretRef maps a secret to a specific file and env-var key within a namespace.
type SecretRef struct {
	Namespace string `toml:"namespace"`
	File      string `toml:"file"` // relative path inside namespace
	Key       string `toml:"key"`  // env var name
}

// SecretFile is the per-secret encrypted TOML document stored at
// vault/secrets/<name>.toml.age. It holds all versions, metadata, and refs.
type SecretFile struct {
	Name      string                   `toml:"name"`
	CreatedBy string                   `toml:"created_by"`
	CreatedAt time.Time                `toml:"created_at"`
	Latest    string                   `toml:"latest"`
	Versions  map[string]SecretVersion `toml:"versions"`
	Refs      []SecretRef              `toml:"refs"`
}

// SecretVersion is an immutable snapshot of a secret value.
type SecretVersion struct {
	CreatedAt time.Time `toml:"created_at"`
	Reason    string    `toml:"reason"`
	Value     []byte    `toml:"value"` // age-encrypted value
}

// LegacySecret is the old inline secret format from VaultState.Secrets.
// Kept for migration only.
type LegacySecret struct {
	Name      string      `toml:"name"`
	Value     []byte      `toml:"value"`
	Refs      []SecretRef `toml:"refs"`
	UpdatedAt time.Time   `toml:"updated_at"`
}

// LegacyVaultState is used to detect and migrate old state files that
// contain secrets inline.
type LegacyVaultState struct {
	Version string                  `toml:"version"`
	Files   map[string]TrackedFile  `toml:"files"`
	Secrets map[string]LegacySecret `toml:"secrets"`
}

// DirtyStatus describes the local state of a tracked file relative to the vault.
type DirtyStatus string

const (
	StatusClean   DirtyStatus = "clean"
	StatusDirty   DirtyStatus = "dirty"
	StatusNew     DirtyStatus = "new"
	StatusMissing DirtyStatus = "missing"
)

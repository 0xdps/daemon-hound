package models

import "time"

// FileMode indicates whether a tracked file is synced across machines or backup-only.
type FileMode string

const (
	ModeSync   FileMode = "sync"
	ModeBackup FileMode = "backup"
)

// MachineConfig holds per-machine DaemonHound configuration.
type MachineConfig struct {
	MachineID    string            `toml:"machine_id"`
	VaultRemote  string            `toml:"vault_remote,omitempty"`
	IdentitySalt string            `toml:"identity_salt,omitempty"` // prefix@postfix salt for identity encryption (global across machines)
	Bindings     map[string]string `toml:"bindings"`                // namespace -> local absolute path
	PendingPush  bool              `toml:"pending_push,omitempty"`  // local vault commits not yet pushed to remote
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
	Secrets map[string]Secret      `toml:"secrets"` // key: secret name
}

// Secret is a named secret with its value encrypted via age.
type Secret struct {
	Name      string      `toml:"name"`
	Value     []byte      `toml:"value"` // age-encrypted value
	Refs      []SecretRef `toml:"refs"`  // where this secret is mapped
	UpdatedAt time.Time   `toml:"updated_at"`
}

// SecretRef maps a secret to a specific file and env-var key within a namespace.
type SecretRef struct {
	Namespace string `toml:"namespace"`
	File      string `toml:"file"` // relative path inside namespace
	Key       string `toml:"key"`  // env var name
}

// DirtyStatus describes the local state of a tracked file relative to the vault.
type DirtyStatus string

const (
	StatusClean   DirtyStatus = "clean"
	StatusDirty   DirtyStatus = "dirty"
	StatusNew     DirtyStatus = "new"
	StatusMissing DirtyStatus = "missing"
)

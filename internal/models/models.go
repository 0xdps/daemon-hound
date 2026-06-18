package models

import "time"

// FileMode indicates whether a tracked file is synced across machines or backup-only.
type FileMode string

const (
	ModeSync   FileMode = "sync"
	ModeBackup FileMode = "backup"
)

// DaemonConfig holds daemon-specific settings.
type DaemonConfig struct {
	WatchInterval    int    `toml:"watch_interval"`     // file watch debounce in seconds (default 2)
	PollInterval     int    `toml:"poll_interval"`      // remote poll interval in seconds (default 30)
	ConflictStrategy string `toml:"conflict_strategy"`  // "local" or "ask" (default "ask")
	MaxLogSize       int    `toml:"max_log_size"`       // max log file size in MB (default 10)
	MaxErrorLogSize  int    `toml:"max_error_log_size"` // max error log file size in MB (default 5)
	LogRetentionDays int    `toml:"log_retention_days"` // keep logs for N days (default 30)
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

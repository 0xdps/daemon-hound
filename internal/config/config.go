package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/BurntSushi/toml"
	"github.com/google/uuid"
)

const (
	AppDirName     = ".dh"
	ConfigFileName = "config.toml"
	IdentityFile   = "identity.age"
)

// Config manages DaemonHound's local machine configuration.
type Config struct {
	path   string
	data   models.MachineConfig
	loaded bool
}

// NewConfig creates a Config manager. Call Load or Init before use.
func NewConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		path: filepath.Join(home, AppDirName, ConfigFileName),
		data: models.MachineConfig{
			Bindings: make(map[string]string),
		},
	}
}

// Load reads the config from disk. Returns an error if the file doesn't exist.
func (c *Config) Load() error {
	if _, err := os.Stat(c.path); os.IsNotExist(err) {
		return fmt.Errorf("config not found at %s: run `dh init` first", c.path)
	}
	if _, err := toml.DecodeFile(c.path, &c.data); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	c.loaded = true
	return nil
}

// Init creates a new config with a generated machine UUID and the given vault remote.
func (c *Config) Init(vaultRemote string) error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	c.data.MachineID = uuid.New().String()
	c.data.VaultRemote = vaultRemote
	c.data.Bindings = make(map[string]string)
	c.loaded = true
	return c.Save()
}

// Save writes the config to disk.
func (c *Config) Save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	f, err := os.OpenFile(c.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(c.data); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}
	return nil
}

// MachineID returns the stable machine UUID.
func (c *Config) MachineID() string {
	return c.data.MachineID
}

// VaultRemote returns the configured vault repository remote URL.
func (c *Config) VaultRemote() string {
	return c.data.VaultRemote
}

// IdentitySalt returns the global identity salt in prefix@postfix format.
func (c *Config) IdentitySalt() string {
	return c.data.IdentitySalt
}

// SetIdentitySalt sets the global identity salt and saves the config.
func (c *Config) SetIdentitySalt(salt string) error {
	c.data.IdentitySalt = salt
	return c.Save()
}

// PendingPush returns true if there are local vault commits not yet pushed to remote.
func (c *Config) PendingPush() bool {
	return c.data.PendingPush
}

// SetPendingPush records whether there are local commits pending a push to remote.
func (c *Config) SetPendingPush(pending bool) error {
	c.data.PendingPush = pending
	return c.Save()
}

// SetBinding records the local absolute path for a namespace.
func (c *Config) SetBinding(namespace, localPath string) error {
	c.data.Bindings[namespace] = localPath
	return c.Save()
}

// GetBinding returns the local path bound to a namespace.
func (c *Config) GetBinding(namespace string) (string, bool) {
	path, ok := c.data.Bindings[namespace]
	return path, ok
}

// Bindings returns all namespace bindings.
func (c *Config) Bindings() map[string]string {
	out := make(map[string]string, len(c.data.Bindings))
	for k, v := range c.data.Bindings {
		out[k] = v
	}
	return out
}

// AppDir returns the absolute path to ~/.dh.
func AppDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, AppDirName)
}

// IdentityPath returns the path to the age identity file.
func IdentityPath() string {
	return filepath.Join(AppDir(), IdentityFile)
}

// VaultPath returns the path to the local vault clone.
func VaultPath() string {
	return filepath.Join(AppDir(), "vault")
}

// StateFilePath returns the path to the encrypted vault state file.
func StateFilePath() string {
	return filepath.Join(VaultPath(), "state.toml.age")
}

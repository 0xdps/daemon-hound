package cmd

import (
	"fmt"
	"os"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/keychain"
	"github.com/0xdps/daemon-hound/internal/storage"
	"github.com/0xdps/daemon-hound/internal/utils"
)

// loadVault loads the user's vault identity from disk and returns a Vault.
// It reads the encrypted identity file, decrypts it with the master password
// (from keychain or prompt), and parses the age identity.
func loadVault() (*storage.Vault, error) {
	cfg := config.NewConfig()
	if err := cfg.Load(); err != nil {
		return nil, fmt.Errorf("not initialized")
	}

	password, err := keychain.Retrieve()
	if err != nil {
		return nil, fmt.Errorf("password not in keychain")
	}

	encIdentity, err := os.ReadFile(config.IdentityPath())
	if err != nil {
		return nil, fmt.Errorf("read identity: %w", err)
	}
	identityStr, err := utils.DecryptWithPassword(string(encIdentity), password, cfg.IdentitySalt())
	if err != nil {
		return nil, fmt.Errorf("decrypt identity: %w", err)
	}
	identity, err := storage.ParseIdentity(string(identityStr))
	if err != nil {
		return nil, fmt.Errorf("parse identity: %w", err)
	}

	return storage.NewVault(config.VaultPath(), identity), nil
}

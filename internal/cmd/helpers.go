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

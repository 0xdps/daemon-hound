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

package keychain

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	serviceName = "daemon-hound"
	accountName = "master-password"
)

// Store saves the master password to the OS keychain.
func Store(password string) error {
	return keyring.Set(serviceName, accountName, password)
}

// Retrieve gets the master password from the OS keychain.
// Returns empty string if not found.
func Retrieve() (string, error) {
	return keyring.Get(serviceName, accountName)
}

// Delete removes the master password from the OS keychain.
func Delete() error {
	return keyring.Delete(serviceName, accountName)
}

// IsSet returns true if a password is stored in the keychain.
func IsSet() bool {
	_, err := Retrieve()
	return err == nil
}

// PromptAndStore asks for the password once and stores it in the keychain.
func PromptAndStore(promptFn func(string) (string, error)) (string, error) {
	password, err := promptFn("Enter master password (will be stored in your OS keychain):")
	if err != nil {
		return "", err
	}
	if password == "" {
		return "", fmt.Errorf("master password cannot be empty")
	}
	if err := Store(password); err != nil {
		return "", fmt.Errorf("failed to store password in keychain: %w", err)
	}
	return password, nil
}

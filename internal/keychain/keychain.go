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

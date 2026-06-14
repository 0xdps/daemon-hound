package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	saltSize    = 32
	prefixSize  = 16 // prefix part of prefix@postfix salt
	postfixSize = 16 // postfix part of prefix@postfix salt
	nonceSize   = 12
	scryptN     = 32768
	scryptR     = 8
	scryptP     = 1
	keyLen      = 32
)

// GenerateIdentitySalt generates a new identity salt in prefix@postfix format.
// Returns base64(16-byte-prefix)@base64(16-byte-postfix).
func GenerateIdentitySalt() (string, error) {
	prefix := make([]byte, prefixSize)
	if _, err := io.ReadFull(rand.Reader, prefix); err != nil {
		return "", fmt.Errorf("failed to generate salt prefix: %w", err)
	}
	postfix := make([]byte, postfixSize)
	if _, err := io.ReadFull(rand.Reader, postfix); err != nil {
		return "", fmt.Errorf("failed to generate salt postfix: %w", err)
	}
	return base64.StdEncoding.EncodeToString(prefix) + "@" + base64.StdEncoding.EncodeToString(postfix), nil
}

// ParseIdentitySalt parses a prefix@postfix salt string into a 32-byte salt.
func ParseIdentitySalt(identitySalt string) ([]byte, error) {
	parts := strings.Split(identitySalt, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid identity salt format: expected prefix@postfix")
	}
	prefix, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("failed to decode salt prefix: %w", err)
	}
	postfix, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode salt postfix: %w", err)
	}
	if len(prefix) != prefixSize || len(postfix) != postfixSize {
		return nil, fmt.Errorf("invalid salt size: expected %d@%d bytes, got %d@%d", prefixSize, postfixSize, len(prefix), len(postfix))
	}
	// Combine prefix + postfix to form 32-byte salt
	salt := append(prefix, postfix...)
	return salt, nil
}

// EncryptWithPassword encrypts data using AES-256-GCM with a key derived from
// password and global identity salt via scrypt.
// The identitySalt must be in prefix@postfix format (from GenerateIdentitySalt).
// Output format: base64(nonce (12 bytes) || ciphertext)
func EncryptWithPassword(plaintext []byte, password string, identitySalt string) (string, error) {
	// Parse global identity salt (prefix@postfix format)
	salt, err := ParseIdentitySalt(identitySalt)
	if err != nil {
		return "", err
	}

	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return "", fmt.Errorf("scrypt key derivation failed: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptWithPassword decrypts data encrypted with EncryptWithPassword.
// The identitySalt must be in prefix@postfix format (from config).
// Input format: base64(nonce (12 bytes) || ciphertext)
func DecryptWithPassword(ciphertextB64 string, password string, identitySalt string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Parse global identity salt
	salt, err := ParseIdentitySalt(identitySalt)
	if err != nil {
		return nil, err
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return nil, fmt.Errorf("scrypt key derivation failed: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong password?): %w", err)
	}

	return plaintext, nil
}

// VerifyPassword checks if the provided password can decrypt the given ciphertext.
// It returns true if decryption succeeds, false otherwise.
func VerifyPassword(ciphertextB64 string, password string, identitySalt string) bool {
	_, err := DecryptWithPassword(ciphertextB64, password, identitySalt)
	return err == nil
}

// ConstantTimeCompare compares two strings in constant time to prevent timing attacks.
func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

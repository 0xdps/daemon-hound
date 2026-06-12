package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/scrypt"
)

const (
	saltSize  = 32
	nonceSize = 12
	scryptN   = 32768
	scryptR   = 8
	scryptP   = 1
	keyLen    = 32
)

// EncryptWithPassword encrypts data using AES-256-GCM with a key derived from
// password via scrypt. The output format is base64-encoded:
// salt (32 bytes) || nonce (12 bytes) || ciphertext.
func EncryptWithPassword(plaintext []byte, password string) (string, error) {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
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
	// Prepend salt
	out := append(salt, ciphertext...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// DecryptWithPassword decrypts data encrypted with EncryptWithPassword.
func DecryptWithPassword(ciphertextB64 string, password string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	if len(data) < saltSize+nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	salt := data[:saltSize]
	nonce := data[saltSize : saltSize+nonceSize]
	ciphertext := data[saltSize+nonceSize:]

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
func VerifyPassword(ciphertextB64 string, password string) bool {
	_, err := DecryptWithPassword(ciphertextB64, password)
	return err == nil
}

// ConstantTimeCompare compares two strings in constant time to prevent timing attacks.
func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

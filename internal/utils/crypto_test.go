package utils

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptWithPassword(t *testing.T) {
	plaintext := []byte("my secret age identity key")
	password := "correct horse battery staple"

	// Encrypt
	ciphertext, err := EncryptWithPassword(plaintext, password)
	if err != nil {
		t.Fatalf("EncryptWithPassword failed: %v", err)
	}
	if ciphertext == "" {
		t.Error("EncryptWithPassword returned empty string")
	}

	// Decrypt with correct password
	decrypted, err := DecryptWithPassword(ciphertext, password)
	if err != nil {
		t.Fatalf("DecryptWithPassword with correct password failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("DecryptWithPassword returned wrong plaintext: got %q, want %q", decrypted, plaintext)
	}

	// Decrypt with wrong password should fail
	_, err = DecryptWithPassword(ciphertext, "wrong password")
	if err == nil {
		t.Error("DecryptWithPassword with wrong password should have failed")
	}
}

func TestEncryptWithPasswordDifferentOutputs(t *testing.T) {
	plaintext := []byte("same plaintext")
	password := "same password"

	// Two encryptions of the same plaintext should produce different outputs
	// due to random salt and nonce
	ciphertext1, err := EncryptWithPassword(plaintext, password)
	if err != nil {
		t.Fatalf("EncryptWithPassword failed: %v", err)
	}
	ciphertext2, err := EncryptWithPassword(plaintext, password)
	if err != nil {
		t.Fatalf("EncryptWithPassword failed: %v", err)
	}

	if ciphertext1 == ciphertext2 {
		t.Error("EncryptWithPassword should produce different outputs for same input due to random salt/nonce")
	}

	// But both should decrypt to the same plaintext
	decrypted1, err := DecryptWithPassword(ciphertext1, password)
	if err != nil {
		t.Fatalf("DecryptWithPassword failed: %v", err)
	}
	decrypted2, err := DecryptWithPassword(ciphertext2, password)
	if err != nil {
		t.Fatalf("DecryptWithPassword failed: %v", err)
	}

	if !bytes.Equal(decrypted1, plaintext) || !bytes.Equal(decrypted2, plaintext) {
		t.Error("Decryption did not return original plaintext")
	}
}

func TestVerifyPassword(t *testing.T) {
	plaintext := []byte("test data")
	password := "my password"

	ciphertext, err := EncryptWithPassword(plaintext, password)
	if err != nil {
		t.Fatalf("EncryptWithPassword failed: %v", err)
	}

	if !VerifyPassword(ciphertext, password) {
		t.Error("VerifyPassword with correct password should return true")
	}

	if VerifyPassword(ciphertext, "wrong password") {
		t.Error("VerifyPassword with wrong password should return false")
	}
}

func TestConstantTimeCompare(t *testing.T) {
	if !ConstantTimeCompare("same", "same") {
		t.Error("ConstantTimeCompare with equal strings should return true")
	}

	if ConstantTimeCompare("different", "strings") {
		t.Error("ConstantTimeCompare with different strings should return false")
	}

	if !ConstantTimeCompare("", "") {
		t.Error("ConstantTimeCompare with empty strings should return true")
	}
}

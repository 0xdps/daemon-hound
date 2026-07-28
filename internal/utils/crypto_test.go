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

package utils

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptWithPassword(t *testing.T) {
	plaintext := []byte("my secret age identity key")
	password := "correct horse battery staple"

	// Generate global salt
	globalSalt, err := GenerateIdentitySalt()
	if err != nil {
		t.Fatalf("GenerateIdentitySalt failed: %v", err)
	}

	// Encrypt with global salt
	ciphertext, err := EncryptWithPassword(plaintext, password, globalSalt)
	if err != nil {
		t.Fatalf("EncryptWithPassword failed: %v", err)
	}
	if ciphertext == "" {
		t.Error("EncryptWithPassword returned empty string")
	}

	// Decrypt with correct password and global salt
	decrypted, err := DecryptWithPassword(ciphertext, password, globalSalt)
	if err != nil {
		t.Fatalf("DecryptWithPassword with correct password failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("DecryptWithPassword returned wrong plaintext: got %q, want %q", decrypted, plaintext)
	}

	// Decrypt with wrong password should fail
	_, err = DecryptWithPassword(ciphertext, "wrong password", globalSalt)
	if err == nil {
		t.Error("DecryptWithPassword with wrong password should have failed")
	}

	// Decrypt with wrong salt should fail
	wrongSalt, _ := GenerateIdentitySalt()
	_, err = DecryptWithPassword(ciphertext, password, wrongSalt)
	if err == nil {
		t.Error("DecryptWithPassword with wrong salt should have failed")
	}
}

func TestGenerateAndParseIdentitySalt(t *testing.T) {
	// Generate salt
	salt, err := GenerateIdentitySalt()
	if err != nil {
		t.Fatalf("GenerateIdentitySalt failed: %v", err)
	}

	// Verify format (should be prefix@postfix)
	if len(salt) == 0 {
		t.Error("GenerateIdentitySalt returned empty string")
	}

	// Parse salt
	saltBytes, err := ParseIdentitySalt(salt)
	if err != nil {
		t.Fatalf("ParseIdentitySalt failed: %v", err)
	}

	// Verify length (16 + 16 = 32 bytes)
	if len(saltBytes) != 32 {
		t.Errorf("ParseIdentitySalt returned wrong length: got %d, want 32", len(saltBytes))
	}
}

func TestEncryptWithPasswordDifferentOutputs(t *testing.T) {
	plaintext := []byte("same plaintext")
	password := "same password"
	globalSalt, _ := GenerateIdentitySalt()

	// With global salt, same password produces different ciphertext (random nonce)
	ciphertext1, err := EncryptWithPassword(plaintext, password, globalSalt)
	if err != nil {
		t.Fatalf("EncryptWithPassword failed: %v", err)
	}
	ciphertext2, err := EncryptWithPassword(plaintext, password, globalSalt)
	if err != nil {
		t.Fatalf("EncryptWithPassword failed: %v", err)
	}

	// Different due to random nonce
	if ciphertext1 == ciphertext2 {
		t.Error("EncryptWithPassword should produce different outputs due to random nonce")
	}

	// But both should decrypt to the same plaintext
	decrypted1, err := DecryptWithPassword(ciphertext1, password, globalSalt)
	if err != nil {
		t.Fatalf("DecryptWithPassword failed: %v", err)
	}
	decrypted2, err := DecryptWithPassword(ciphertext2, password, globalSalt)
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
	globalSalt, _ := GenerateIdentitySalt()

	ciphertext, err := EncryptWithPassword(plaintext, password, globalSalt)
	if err != nil {
		t.Fatalf("EncryptWithPassword failed: %v", err)
	}

	if !VerifyPassword(ciphertext, password, globalSalt) {
		t.Error("VerifyPassword with correct password should return true")
	}

	if VerifyPassword(ciphertext, "wrong password", globalSalt) {
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

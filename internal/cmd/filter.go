package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var encryptCmd = &cobra.Command{
	Use:   "encrypt",
	Short: "Git clean filter: encrypt stdin to stdout",
	Long: `Acts as a Git clean filter for the "dhd" filter.

Git calls this during staging:
  dhd encrypt < plaintext > ciphertext

Reads plaintext from stdin, encrypts it with the vault identity,
and writes ciphertext to stdout.

This is typically configured as:
  [filter "dhd"]
      clean = dhd encrypt
      smudge = dhd decrypt`,
	Args: cobra.NoArgs,
	RunE: runEncrypt,
}

var decryptCmd = &cobra.Command{
	Use:   "decrypt",
	Short: "Git smudge filter: decrypt stdin to stdout",
	Long: `Acts as a Git smudge filter for the "dhd" filter.

Git calls this during checkout:
  dhd decrypt < ciphertext > plaintext

Reads ciphertext from stdin, decrypts it with the vault identity,
and writes plaintext to stdout.

This is typically configured as:
  [filter "dhd"]
      clean = dhd encrypt
      smudge = dhd decrypt`,
	Args: cobra.NoArgs,
	RunE: runDecrypt,
}

func init() {
	rootCmd.AddCommand(encryptCmd)
	rootCmd.AddCommand(decryptCmd)
}

func runEncrypt(cmd *cobra.Command, args []string) error {
	plaintext, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	vault, err := loadVault()
	if err != nil {
		return fmt.Errorf("load vault: %w", err)
	}

	ciphertext, err := vault.Encrypt(plaintext)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	if _, err := os.Stdout.Write(ciphertext); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	return nil
}

func runDecrypt(cmd *cobra.Command, args []string) error {
	ciphertext, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	vault, err := loadVault()
	if err != nil {
		return fmt.Errorf("load vault: %w", err)
	}

	plaintext, err := vault.Decrypt(ciphertext)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}

	if _, err := os.Stdout.Write(plaintext); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	return nil
}

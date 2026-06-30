package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var diffTextconv bool

var diffCmd = &cobra.Command{
	Use:   "diff [old-file] [new-file]",
	Short: "Git diff driver for encrypted vault files",
	Long: `Acts as a Git diff driver for encrypted vault files.

When called by Git as a diff driver (via diff.dhd.textconv):
  dhd diff --textconv <file>

When called manually:
  dhd diff old.age new.age

Shows the decrypted content diff instead of "Binary files differ".

Git configuration:
  [diff "dhd"]
      textconv = dhd diff --textconv

Then in .gitattributes:
  *.age diff=dhd`,
	Args: cobra.RangeArgs(0, 2),
	RunE: runDiff,
}

func init() {
	diffCmd.Flags().BoolVar(&diffTextconv, "textconv", false, "Git textconv mode: single file argument")
	rootCmd.AddCommand(diffCmd)
}

func runDiff(cmd *cobra.Command, args []string) error {
	if diffTextconv {
		// Git textconv mode: single file path.
		if len(args) != 1 {
			return fmt.Errorf("textconv mode requires exactly one file argument")
		}
		return diffTextconvFile(args[0])
	}

	// Manual mode: two file paths.
	if len(args) != 2 {
		return fmt.Errorf("manual diff requires two file arguments: dhd diff old.age new.age")
	}
	return diffTwoFiles(args[0], args[1])
}

func diffTextconvFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	// If it looks like an age-encrypted file, try to decrypt.
	if isAgeEncrypted(data) {
		vault, err := loadVault()
		if err != nil {
			// Can't decrypt — show a placeholder so Git doesn't show binary.
			fmt.Printf("# [encrypted: %s]\n", path)
			return nil
		}
		plain, err := vault.Decrypt(data)
		if err != nil {
			fmt.Printf("# [decryption failed: %s]\n", path)
			return nil
		}
		os.Stdout.Write(plain)
		return nil
	}

	// Not encrypted — just print the file content.
	os.Stdout.Write(data)
	return nil
}

func diffTwoFiles(oldPath, newPath string) error {
	oldData, err := os.ReadFile(oldPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", oldPath, err)
	}
	newData, err := os.ReadFile(newPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", newPath, err)
	}

	oldPlain, err := decryptIfEncrypted(oldData)
	if err != nil {
		return fmt.Errorf("decrypt %s: %w", oldPath, err)
	}
	newPlain, err := decryptIfEncrypted(newData)
	if err != nil {
		return fmt.Errorf("decrypt %s: %w", newPath, err)
	}

	// Simple line-by-line diff output.
	oldLines := splitLines(string(oldPlain))
	newLines := splitLines(string(newPlain))

	// Very basic unified diff header.
	fmt.Printf("--- %s\n", oldPath)
	fmt.Printf("+++ %s\n", newPath)

	// For now, just print both sides with +/- markers.
	// A real diff algorithm is overkill for this use case.
	for _, line := range oldLines {
		fmt.Printf("-%s\n", line)
	}
	for _, line := range newLines {
		fmt.Printf("+%s\n", line)
	}

	return nil
}

// decryptIfEncrypted attempts to decrypt data if it looks age-encrypted.
// Returns the data as-is if not encrypted.
func decryptIfEncrypted(data []byte) ([]byte, error) {
	if !isAgeEncrypted(data) {
		return data, nil
	}
	vault, err := loadVault()
	if err != nil {
		return nil, err
	}
	return vault.Decrypt(data)
}

// isAgeEncrypted checks if data appears to be an age-encrypted file.
// Age files start with "age-encryption.org/v1".
func isAgeEncrypted(data []byte) bool {
	if len(data) < 20 {
		return false
	}
	return string(data[:20]) == "age-encryption.org/v1"
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

package cmd

import (
	"fmt"
	"os"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/storage"
	"github.com/0xdps/daemon-hound/internal/utils"
	"github.com/spf13/cobra"
)

var initRemote string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize DaemonHound with a vault repository",
	Long: `Initialize DaemonHound on this machine.

This command:
1. Generates a stable machine UUID
2. Generates an age identity key (encrypted with your master password)
3. Clones (or initializes) the vault repository
4. Stores configuration in ~/.dh/`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&initRemote, "remote", "", "Git URL of the private vault repository")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cfg := config.NewConfig()

	// Check if already initialized
	if err := cfg.Load(); err == nil {
		return fmt.Errorf("daemon-hound already initialized (machine_id: %s)", cfg.MachineID())
	}

	// Prompt for remote if not provided
	if initRemote == "" {
		remote, err := utils.PromptInput("Vault repository URL (e.g. git@github.com:you/vault.git):")
		if err != nil {
			return err
		}
		initRemote = remote
	}

	// Initialize config
	if err := cfg.Init(); err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	// Prompt for master password
	password, err := utils.PromptPassword("Enter master password (used to encrypt your identity key):")
	if err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("master password cannot be empty")
	}

	// Generate age identity
	identity, err := storage.GenerateIdentity()
	if err != nil {
		return fmt.Errorf("failed to generate age identity: %w", err)
	}

	// Encrypt identity with password (simple XOR-based obfuscation for now; in production use scrypt+AES)
	encryptedIdentity := encryptWithPassword([]byte(identity.String()), password)
	if err := os.WriteFile(config.IdentityPath(), encryptedIdentity, 0600); err != nil {
		return fmt.Errorf("failed to write identity file: %w", err)
	}

	// Clone or init vault
	vaultPath := config.VaultPath()
	if _, err := os.Stat(vaultPath); !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "Vault directory already exists, reusing.")
	} else {
		if err := git.Clone(initRemote, vaultPath); err != nil {
			fmt.Fprintf(os.Stderr, "Clone failed (%v), initializing fresh vault...\n", err)
			if err := git.Init(vaultPath); err != nil {
				return fmt.Errorf("failed to initialize vault: %w", err)
			}
			gitClient := git.NewClient(vaultPath)
			if err := gitClient.AddRemote(initRemote); err != nil {
				return fmt.Errorf("failed to add remote: %w", err)
			}
		}
	}

	fmt.Printf("DaemonHound initialized.\n")
	fmt.Printf("Machine ID: %s\n", cfg.MachineID())
	fmt.Printf("Vault:      %s\n", vaultPath)
	fmt.Printf("\n⚠️  Back up %s immediately. Without it, encrypted vault data cannot be recovered.\n", config.IdentityPath())
	return nil
}

// encryptWithPassword applies a simple password-based encryption.
// TODO: replace with scrypt + AES-GCM or similar in production.
func encryptWithPassword(data []byte, password string) []byte {
	out := make([]byte, len(data))
	for i := range data {
		out[i] = data[i] ^ password[i%len(password)]
	}
	return out
}

// decryptWithPassword decrypts data encrypted with encryptWithPassword.
func decryptWithPassword(data []byte, password string) []byte {
	return encryptWithPassword(data, password) // XOR is symmetric
}

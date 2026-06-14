package cmd

import (
	"fmt"
	"os"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/daemon"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/keychain"
	"github.com/0xdps/daemon-hound/internal/storage"
	"github.com/0xdps/daemon-hound/internal/utils"
	"github.com/spf13/cobra"
)

var initRemote string
var initForce bool

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
	initCmd.Flags().BoolVar(&initForce, "force", false, "Re-initialize even if already set up on this machine")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cfg := config.NewConfig()

	// Check if already initialized
	if err := cfg.Load(); err == nil {
		if !initForce {
			return fmt.Errorf("daemon-hound already initialized (machine_id: %s). Use --force to re-initialize", cfg.MachineID())
		}
		fmt.Fprintf(os.Stderr, "Warning: re-initializing — previous machine_id %s will be replaced.\n", cfg.MachineID())
	}

	// Prompt for remote if not provided
	if initRemote == "" {
		// Pre-fill with existing remote if re-initializing
		defaultRemote := ""
		if initForce {
			_ = cfg.Load()
			defaultRemote = cfg.VaultRemote()
		}
		promptMsg := "Vault repository URL (e.g. git@github.com:you/vault.git):"
		if defaultRemote != "" {
			promptMsg = fmt.Sprintf("Vault repository URL [%s]:", defaultRemote)
		}
		remote, err := utils.PromptInput(promptMsg)
		if err != nil {
			return err
		}
		if remote == "" && defaultRemote != "" {
			remote = defaultRemote
		}
		initRemote = remote
	}

	// Initialize config (saves machine UUID and vault remote)
	if err := cfg.Init(initRemote); err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	// Generate global identity salt (prefix@postfix) if not already set
	if cfg.IdentitySalt() == "" {
		identitySalt, err := utils.GenerateIdentitySalt()
		if err != nil {
			return fmt.Errorf("failed to generate identity salt: %w", err)
		}
		if err := cfg.SetIdentitySalt(identitySalt); err != nil {
			return fmt.Errorf("failed to save identity salt: %w", err)
		}
	}

	// Prompt for master password with confirmation
	password, err := utils.PromptPassword("Enter master password (used to encrypt your identity key):")
	if err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("master password cannot be empty")
	}
	confirm, err := utils.PromptPassword("Confirm master password:")
	if err != nil {
		return err
	}
	if password != confirm {
		return fmt.Errorf("passwords do not match")
	}

	// Generate age identity
	identity, err := storage.GenerateIdentity()
	if err != nil {
		return fmt.Errorf("failed to generate age identity: %w", err)
	}

	// Encrypt identity with password using global salt (scrypt+AES-GCM)
	encryptedIdentity, err := utils.EncryptWithPassword([]byte(identity.String()), password, cfg.IdentitySalt())
	if err != nil {
		return fmt.Errorf("failed to encrypt identity: %w", err)
	}
	if err := os.WriteFile(config.IdentityPath(), []byte(encryptedIdentity), 0600); err != nil {
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

	// Store password in keychain for future use
	if err := keychain.Store(password); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to store password in keychain: %v\n", err)
	}

	// Install daemon service for background syncing
	sm := daemon.NewServiceManager()
	if err := sm.Install(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to install daemon service: %v\n", err)
		fmt.Fprintf(os.Stderr, "You can manually start syncing with: dh sync\n")
	} else {
		fmt.Println("✓ Background sync daemon installed")
		fmt.Println("  Auto-starts on system boot")
		fmt.Println("  Syncs every 30 seconds")
	}

	fmt.Printf("\nDaemonHound initialized.\n")
	fmt.Printf("Machine ID: %s\n", cfg.MachineID())
	fmt.Printf("Vault:      %s\n", vaultPath)
	fmt.Printf("\n⚠️  Back up %s immediately. Without it, encrypted vault data cannot be recovered.\n", config.IdentityPath())
	return nil
}

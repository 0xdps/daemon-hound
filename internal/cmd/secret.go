package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/0xdps/daemon-hound/internal/storage"
	"github.com/0xdps/daemon-hound/internal/utils"
	"github.com/spf13/cobra"
)

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Manage named secrets",
	Long:  `Store, retrieve, and map named secrets across repositories.`,
}

var secretSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Store or update a named secret",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretSet,
}

var secretGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Retrieve a named secret",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretGet,
}

var secretListCmd = &cobra.Command{
	Use:   "list [name]",
	Short: "List all secrets or mappings for a specific secret",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSecretList,
}

var secretRefCmd = &cobra.Command{
	Use:   "ref <name> <file> <key>",
	Short: "Map a secret to a file and env-var key in the current repo",
	Long: `Map a secret to a specific file and environment variable key.

Example:
  dh secret ref openai-key .env.local OPENAI_API_KEY`,
	Args: cobra.ExactArgs(3),
	RunE: runSecretRef,
}

func init() {
	secretCmd.AddCommand(secretSetCmd)
	secretCmd.AddCommand(secretGetCmd)
	secretCmd.AddCommand(secretListCmd)
	secretCmd.AddCommand(secretRefCmd)
	rootCmd.AddCommand(secretCmd)
}

func runSecretSet(cmd *cobra.Command, args []string) error {
	name := args[0]
	_, vault, _, err := loadContext()
	if err != nil {
		return err
	}

	value, err := utils.PromptPassword("Enter secret value:")
	if err != nil {
		return err
	}
	if value == "" {
		return fmt.Errorf("secret value cannot be empty")
	}

	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load vault state: %w", err)
	}

	encValue, err := vault.Encrypt([]byte(value))
	if err != nil {
		return fmt.Errorf("failed to encrypt secret: %w", err)
	}

	secret, exists := state.Secrets[name]
	if !exists {
		secret = models.Secret{Name: name}
	}
	secret.Value = encValue
	secret.UpdatedAt = time.Now()
	state.Secrets[name] = secret

	if err := vault.SaveState(state); err != nil {
		return fmt.Errorf("failed to save vault state: %w", err)
	}

	// Update all referenced files
	updatedCount := 0
	for _, ref := range secret.Refs {
		if err := updateFileWithSecret(vault, ref, value); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update %s/%s: %v\n", ref.Namespace, ref.File, err)
			continue
		}
		updatedCount++
	}

	// Commit and push
	gitClient := git.NewClient(config.VaultPath())
	if err := gitClient.CommitAll(fmt.Sprintf("daemon-hound: update secret %s", name)); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	if err := gitClient.Push(); err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	fmt.Printf("Stored secret: %s\n", name)
	if updatedCount > 0 {
		fmt.Printf("Updated %d file(s)\n", updatedCount)
	}
	return nil
}

func runSecretGet(cmd *cobra.Command, args []string) error {
	name := args[0]
	_, vault, _, err := loadContext()
	if err != nil {
		return err
	}

	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load vault state: %w", err)
	}

	secret, ok := state.Secrets[name]
	if !ok {
		return fmt.Errorf("secret not found: %s", name)
	}

	value, err := vault.Decrypt(secret.Value)
	if err != nil {
		return fmt.Errorf("failed to decrypt secret: %w", err)
	}

	fmt.Println(string(value))
	return nil
}

func runSecretList(cmd *cobra.Command, args []string) error {
	_, vault, _, err := loadContext()
	if err != nil {
		return err
	}

	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load vault state: %w", err)
	}

	if len(args) == 1 {
		// List mappings for a specific secret
		name := args[0]
		secret, ok := state.Secrets[name]
		if !ok {
			return fmt.Errorf("secret not found: %s", name)
		}
		fmt.Printf("%s\n", name)
		for _, ref := range secret.Refs {
			fmt.Printf("  %-30s  %-20s  %s\n", ref.Namespace, ref.File, ref.Key)
		}
		return nil
	}

	// List all secrets
	if len(state.Secrets) == 0 {
		fmt.Println("No secrets stored.")
		return nil
	}
	for name := range state.Secrets {
		fmt.Println(name)
	}
	return nil
}

func runSecretRef(cmd *cobra.Command, args []string) error {
	name := args[0]
	filePath := args[1]
	key := args[2]

	cfg, vault, _, err := loadContext()
	if err != nil {
		return err
	}

	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load vault state: %w", err)
	}

	secret, ok := state.Secrets[name]
	if !ok {
		return fmt.Errorf("secret not found: %s", name)
	}

	// Determine namespace from current directory
	repoRoot, err := utils.FindGitRoot(".")
	if err != nil {
		return fmt.Errorf("not inside a git repository: %w", err)
	}
	origin, err := utils.GetGitOrigin(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to get git origin: %w", err)
	}
	namespace, err := utils.DeriveNamespace(origin)
	if err != nil {
		return fmt.Errorf("failed to derive namespace: %w", err)
	}

	// Record binding if not already set
	if _, ok := cfg.GetBinding(namespace); !ok {
		if err := cfg.SetBinding(namespace, repoRoot); err != nil {
			return fmt.Errorf("failed to save binding: %w", err)
		}
	}

	// Add or update ref
	ref := models.SecretRef{Namespace: namespace, File: filePath, Key: key}
	found := false
	for i, existing := range secret.Refs {
		if existing.Namespace == namespace && existing.File == filePath {
			secret.Refs[i] = ref
			found = true
			break
		}
	}
	if !found {
		secret.Refs = append(secret.Refs, ref)
	}
	state.Secrets[name] = secret

	if err := vault.SaveState(state); err != nil {
		return fmt.Errorf("failed to save vault state: %w", err)
	}

	// Write current secret value into the file
	value, err := vault.Decrypt(secret.Value)
	if err != nil {
		return fmt.Errorf("failed to decrypt secret: %w", err)
	}
	if err := updateFileWithSecret(vault, ref, string(value)); err != nil {
		return fmt.Errorf("failed to update file: %w", err)
	}

	fmt.Printf("Mapped: %s → %s:%s:%s\n", name, namespace, filePath, key)
	return nil
}

// updateFileWithSecret writes or updates a key=value line in the target file.
func updateFileWithSecret(vault *storage.Vault, ref models.SecretRef, value string) error {
	cfg := config.NewConfig()
	if err := cfg.Load(); err != nil {
		return err
	}

	root, ok := cfg.GetBinding(ref.Namespace)
	if !ok {
		return fmt.Errorf("no binding for namespace %s", ref.Namespace)
	}

	localPath := fmt.Sprintf("%s/%s", root, ref.File)
	var lines []string
	if data, err := os.ReadFile(localPath); err == nil {
		lines = strings.Split(string(data), "\n")
	}

	prefix := ref.Key + "="
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[i] = prefix + value
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, prefix+value)
	}

	output := strings.Join(lines, "\n")
	if err := os.WriteFile(localPath, []byte(output), 0600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

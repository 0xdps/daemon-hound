package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xdps/daemon-hound/internal/audit"
	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/models"
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

var secretDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a named secret and all its mappings",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretDelete,
}

var secretUnrefCmd = &cobra.Command{
	Use:   "unref <name> <file>",
	Short: "Remove a secret mapping from the current repo",
	Long: `Remove the mapping between a secret and a file in the current repository.

The file is not modified — only the mapping is removed.

Example:
  dh secret unref openai-key .env.local`,
	Args: cobra.ExactArgs(2),
	RunE: runSecretUnref,
}

var secretRenameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename a secret",
	Long: `Rename a secret key. The value and all mappings are preserved.

Example:
  dh secret rename openai-key openai-prod-key`,
	Args: cobra.ExactArgs(2),
	RunE: runSecretRename,
}

func init() {
	secretCmd.AddCommand(secretSetCmd)
	secretCmd.AddCommand(secretGetCmd)
	secretCmd.AddCommand(secretListCmd)
	secretCmd.AddCommand(secretRefCmd)
	secretCmd.AddCommand(secretDeleteCmd)
	secretCmd.AddCommand(secretUnrefCmd)
	secretCmd.AddCommand(secretRenameCmd)
	rootCmd.AddCommand(secretCmd)
}

func runSecretSet(cmd *cobra.Command, args []string) error {
	defer mustLock()()
	name := args[0]
	cfg, vault, _, err := loadContext()
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

	encValue, err := vault.Encrypt([]byte(value))
	if err != nil {
		return fmt.Errorf("failed to encrypt secret: %w", err)
	}

	gitClient := git.NewClient(config.VaultPath())
	if err := vaultCommitPush(gitClient, vault, fmt.Sprintf("daemon-hound: update secret %s", name), func(state *models.VaultState) error {
		secret, exists := state.Secrets[name]
		if !exists {
			secret = models.Secret{Name: name}
		}
		secret.Value = encValue
		secret.UpdatedAt = time.Now()
		state.Secrets[name] = secret
		return nil
	}); err != nil {
		return err
	}

	// Update all referenced files with per-file output (read state fresh after commit)
	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("failed to reload vault state: %w", err)
	}
	updatedCount := 0
	if secret, ok := state.Secrets[name]; ok {
		for _, ref := range secret.Refs {
			if err := updateFileWithSecret(cfg, ref, value); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: failed to update %s in %s: %v\n", ref.File, ref.Namespace, err)
				continue
			}
			fmt.Printf("  → Updated %s in %s  (%s)\n", ref.File, ref.Namespace, ref.Key)
			updatedCount++
		}
	}

	if updatedCount > 0 {
		fmt.Printf("%d file(s) marked dirty — run `dh sync` to push\n", updatedCount)
	} else {
		fmt.Printf("Stored secret: %s\n", name)
	}
	audit.Log("secret set", fmt.Sprintf("%s refs=%d", name, updatedCount))
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
		if len(secret.Refs) == 0 {
			fmt.Println("  (no mappings)")
			return nil
		}
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
	for name, secret := range state.Secrets {
		fmt.Printf("%-30s  %d mapping(s)\n", name, len(secret.Refs))
	}
	return nil
}

func runSecretRef(cmd *cobra.Command, args []string) error {
	defer mustLock()()
	name := args[0]
	filePath := args[1]
	key := args[2]

	cfg, vault, _, err := loadContext()
	if err != nil {
		return err
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

	ref := models.SecretRef{Namespace: namespace, File: filePath, Key: key}
	var secretValue []byte

	gitClient := git.NewClient(config.VaultPath())
	if err := vaultCommitPush(gitClient, vault, fmt.Sprintf("daemon-hound: ref %s → %s:%s", name, namespace, filePath), func(state *models.VaultState) error {
		secret, ok := state.Secrets[name]
		if !ok {
			return fmt.Errorf("secret not found: %s (use `dh secret set %s` first)", name, name)
		}
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

		val, err := vault.Decrypt(secret.Value)
		if err != nil {
			return fmt.Errorf("failed to decrypt secret: %w", err)
		}
		secretValue = val
		return nil
	}); err != nil {
		return err
	}

	if err := updateFileWithSecret(cfg, ref, string(secretValue)); err != nil {
		return fmt.Errorf("failed to update file: %w", err)
	}

	fmt.Printf("Mapped: %s → %s:%s:%s\n", name, namespace, filePath, key)
	fmt.Println("Run `dh sync` to push the updated file to the vault.")
	audit.Log("secret ref", fmt.Sprintf("%s → %s:%s:%s", name, namespace, filePath, key))
	return nil
}

func runSecretDelete(cmd *cobra.Command, args []string) error {
	defer mustLock()()
	name := args[0]
	_, vault, _, err := loadContext()
	if err != nil {
		return err
	}

	gitClient := git.NewClient(config.VaultPath())
	if err := vaultCommitPush(gitClient, vault, fmt.Sprintf("daemon-hound: delete secret %s", name), func(state *models.VaultState) error {
		if _, ok := state.Secrets[name]; !ok {
			return fmt.Errorf("secret not found: %s", name)
		}
		delete(state.Secrets, name)
		return nil
	}); err != nil {
		return err
	}

	fmt.Printf("Deleted secret: %s\n", name)
	fmt.Println("Note: any files that contained this secret value were NOT modified.")
	audit.Log("secret delete", name)
	return nil
}

func runSecretUnref(cmd *cobra.Command, args []string) error {
	name := args[0]
	filePath := args[1]

	_, vault, _, err := loadContext()
	if err != nil {
		return err
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

	gitClient := git.NewClient(config.VaultPath())
	if err := vaultCommitPush(gitClient, vault, fmt.Sprintf("daemon-hound: unref %s from %s:%s", name, namespace, filePath), func(state *models.VaultState) error {
		secret, ok := state.Secrets[name]
		if !ok {
			return fmt.Errorf("secret not found: %s", name)
		}
		newRefs := secret.Refs[:0]
		removed := false
		for _, ref := range secret.Refs {
			if ref.Namespace == namespace && ref.File == filePath {
				removed = true
				continue
			}
			newRefs = append(newRefs, ref)
		}
		if !removed {
			return fmt.Errorf("no mapping found for %s in %s:%s", name, namespace, filePath)
		}
		secret.Refs = newRefs
		state.Secrets[name] = secret
		return nil
	}); err != nil {
		return err
	}

	fmt.Printf("Removed mapping: %s from %s:%s\n", name, namespace, filePath)
	fmt.Println("Note: the file was not modified — only the mapping was removed.")
	return nil
}

func runSecretRename(cmd *cobra.Command, args []string) error {
	defer mustLock()()
	oldName, newName := args[0], args[1]

	_, vault, _, err := loadContext()
	if err != nil {
		return err
	}

	gitClient := git.NewClient(config.VaultPath())
	if err := vaultCommitPush(gitClient, vault, fmt.Sprintf("daemon-hound: rename secret %s → %s", oldName, newName), func(state *models.VaultState) error {
		secret, ok := state.Secrets[oldName]
		if !ok {
			return fmt.Errorf("secret not found: %s", oldName)
		}
		if _, exists := state.Secrets[newName]; exists {
			return fmt.Errorf("secret already exists: %s", newName)
		}
		secret.Name = newName
		delete(state.Secrets, oldName)
		state.Secrets[newName] = secret
		return nil
	}); err != nil {
		return err
	}

	fmt.Printf("Renamed secret: %s → %s\n", oldName, newName)
	audit.Log("secret rename", fmt.Sprintf("%s → %s", oldName, newName))
	return nil
}

// updateFileWithSecret writes or updates a KEY=value line in the target file.
func updateFileWithSecret(cfg *config.Config, ref models.SecretRef, value string) error {
	root, ok := cfg.GetBinding(ref.Namespace)
	if !ok {
		return fmt.Errorf("no local binding for namespace %s", ref.Namespace)
	}

	localPath := filepath.Join(root, ref.File)
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

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xdps/daemon-hound/internal/audit"
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
	Long:  `Store, retrieve, rotate, and map named secrets across repositories.`,
}

var secretSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Store or update a named secret",
	Long: `Store a new secret or create a new version of an existing secret.

If the secret does not exist, it is created with version v1.
If the secret already exists, a new version is created (immutable history).`,
	Args: cobra.ExactArgs(1),
	RunE: runSecretSet,
}

var secretGetCmd = &cobra.Command{
	Use:   "get <name> [version]",
	Short: "Retrieve a named secret",
	Long: `Retrieve the latest version of a secret, or a specific version.

Examples:
  dhd secret get payment-config-key          # latest version
  dhd secret get payment-config-key v2       # specific version`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runSecretGet,
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
  dhd secret ref openai-key .env.local OPENAI_API_KEY`,
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
  dhd secret unref openai-key .env.local`,
	Args: cobra.ExactArgs(2),
	RunE: runSecretUnref,
}

var secretRenameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename a secret",
	Long: `Rename a secret key. All versions and mappings are preserved.

Example:
  dhd secret rename openai-key openai-prod-key`,
	Args: cobra.ExactArgs(2),
	RunE: runSecretRename,
}

var secretRotateCmd = &cobra.Command{
	Use:   "rotate <name>",
	Short: "Create a new version of a secret",
	Long: `Rotate a secret by creating a new immutable version.

The previous version is preserved. Only the "latest" pointer moves forward.
You will be prompted for a rotation reason (e.g., "PCI compliance", "Leak suspected").

Example:
  dhd secret rotate payment-config-key`,
	Args: cobra.ExactArgs(1),
	RunE: runSecretRotate,
}

var secretRollbackCmd = &cobra.Command{
	Use:   "rollback <name> <version>",
	Short: "Rollback a secret to a previous version",
	Long: `Move the "latest" pointer to a previous version.

No data is deleted — the pointer simply changes.

Example:
  dhd secret rollback payment-config-key v2`,
	Args: cobra.ExactArgs(2),
	RunE: runSecretRollback,
}

var secretHistoryCmd = &cobra.Command{
	Use:   "history <name>",
	Short: "Show version history of a secret",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretHistory,
}

var secretDiffCmd = &cobra.Command{
	Use:   "diff <name> <version-a> <version-b>",
	Short: "Compare two versions of a secret",
	Long: `Show the difference between two secret versions.
Values are decrypted for comparison but not displayed in full.

Example:
  dhd secret diff payment-config-key v1 v3`,
	Args: cobra.ExactArgs(3),
	RunE: runSecretDiff,
}

func init() {
	secretCmd.AddCommand(secretSetCmd)
	secretCmd.AddCommand(secretGetCmd)
	secretCmd.AddCommand(secretListCmd)
	secretCmd.AddCommand(secretRefCmd)
	secretCmd.AddCommand(secretDeleteCmd)
	secretCmd.AddCommand(secretUnrefCmd)
	secretCmd.AddCommand(secretRenameCmd)
	secretCmd.AddCommand(secretRotateCmd)
	secretCmd.AddCommand(secretRollbackCmd)
	secretCmd.AddCommand(secretHistoryCmd)
	secretCmd.AddCommand(secretDiffCmd)
	rootCmd.AddCommand(secretCmd)
}

// nextVersion returns the next version string (v1, v2, ...).
func nextVersion(sf *models.SecretFile) string {
	max := 0
	for v := range sf.Versions {
		var n int
		if _, err := fmt.Sscanf(v, "v%d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("v%d", max+1)
}

// getLatestValue decrypts and returns the value of the latest version.
func getLatestValue(vault *storage.Vault, sf *models.SecretFile) ([]byte, error) {
	ver, ok := sf.Versions[sf.Latest]
	if !ok {
		return nil, fmt.Errorf("latest version %s not found", sf.Latest)
	}
	return vault.Decrypt(ver.Value)
}

// getVersionValue decrypts and returns the value of a specific version.
func getVersionValue(vault *storage.Vault, sf *models.SecretFile, version string) ([]byte, error) {
	ver, ok := sf.Versions[version]
	if !ok {
		return nil, fmt.Errorf("version %s not found", version)
	}
	return vault.Decrypt(ver.Value)
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
		sf, err := vault.LoadSecretFile(name)
		if err != nil {
			// Secret does not exist — create it.
			sf = &models.SecretFile{
				Name:      name,
				CreatedBy: "dhd",
				CreatedAt: time.Now(),
				Latest:    "v1",
				Versions: map[string]models.SecretVersion{
					"v1": {
						CreatedAt: time.Now(),
						Reason:    "Initial",
						Value:     encValue,
					},
				},
			}
		} else {
			// Secret exists — create a new version.
			newVer := nextVersion(sf)
			sf.Versions[newVer] = models.SecretVersion{
				CreatedAt: time.Now(),
				Reason:    "Updated via set",
				Value:     encValue,
			}
			sf.Latest = newVer
		}
		if err := vault.SaveSecretFile(sf); err != nil {
			return err
		}
		state.Secrets[name] = models.SecretIndex{
			Latest:    sf.Latest,
			Versions:  len(sf.Versions),
			UpdatedAt: time.Now(),
		}
		return nil
	}); err != nil {
		return err
	}

	// Update all referenced files with per-file output.
	sf, err := vault.LoadSecretFile(name)
	if err != nil {
		return fmt.Errorf("failed to reload secret: %w", err)
	}
	secretValue, err := getLatestValue(vault, sf)
	if err != nil {
		return fmt.Errorf("failed to decrypt secret: %w", err)
	}

	updatedCount := 0
	for _, ref := range sf.Refs {
		if cfg.IsFileIgnored(ref.Namespace, ref.File) {
			continue
		}
		if err := updateFileWithSecret(cfg, ref, string(secretValue)); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to update %s in %s: %v\n", ref.File, ref.Namespace, err)
			continue
		}
		fmt.Printf("  → Updated %s in %s  (%s)\n", ref.File, ref.Namespace, ref.Key)
		updatedCount++
	}

	if updatedCount > 0 {
		fmt.Printf("%d file(s) marked dirty — run `dhd sync` to push\n", updatedCount)
	} else {
		fmt.Printf("Stored secret: %s (version %s)\n", name, sf.Latest)
	}
	audit.Log("secret set", fmt.Sprintf("%s %s refs=%d", name, sf.Latest, updatedCount))
	return nil
}

func runSecretGet(cmd *cobra.Command, args []string) error {
	name := args[0]
	_, vault, _, err := loadContext()
	if err != nil {
		return err
	}

	sf, err := vault.LoadSecretFile(name)
	if err != nil {
		return err
	}

	version := sf.Latest
	if len(args) == 2 {
		version = args[1]
	}

	value, err := getVersionValue(vault, sf, version)
	if err != nil {
		return err
	}

	fmt.Println(string(value))
	return nil
}

func runSecretList(cmd *cobra.Command, args []string) error {
	_, vault, _, err := loadContext()
	if err != nil {
		return err
	}

	if len(args) == 1 {
		// List mappings for a specific secret
		name := args[0]
		sf, err := vault.LoadSecretFile(name)
		if err != nil {
			return err
		}
		fmt.Printf("%s  (latest: %s, %d version(s))\n", name, sf.Latest, len(sf.Versions))
		if len(sf.Refs) == 0 {
			fmt.Println("  (no mappings)")
			return nil
		}
		for _, ref := range sf.Refs {
			fmt.Printf("  %-30s  %-20s  %s\n", ref.Namespace, ref.File, ref.Key)
		}
		return nil
	}

	// List all secrets — use the lightweight index for speed.
	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load vault state: %w", err)
	}
	if len(state.Secrets) == 0 {
		fmt.Println("No secrets stored.")
		return nil
	}
	for name, idx := range state.Secrets {
		fmt.Printf("%-30s  latest=%-4s  %d version(s)\n", name, idx.Latest, idx.Versions)
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
		sf, err := vault.LoadSecretFile(name)
		if err != nil {
			return fmt.Errorf("secret not found: %s (use `dhd secret set %s` first)", name, name)
		}
		found := false
		for i, existing := range sf.Refs {
			if existing.Namespace == namespace && existing.File == filePath {
				sf.Refs[i] = ref
				found = true
				break
			}
		}
		if !found {
			sf.Refs = append(sf.Refs, ref)
		}
		if err := vault.SaveSecretFile(sf); err != nil {
			return fmt.Errorf("failed to save secret: %w", err)
		}

		val, err := getLatestValue(vault, sf)
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
	fmt.Println("Run `dhd sync` to push the updated file to the vault.")
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
		if err := vault.DeleteSecretFile(name); err != nil {
			return fmt.Errorf("failed to delete secret: %w", err)
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
		sf, err := vault.LoadSecretFile(name)
		if err != nil {
			return err
		}
		newRefs := sf.Refs[:0]
		removed := false
		for _, ref := range sf.Refs {
			if ref.Namespace == namespace && ref.File == filePath {
				removed = true
				continue
			}
			newRefs = append(newRefs, ref)
		}
		if !removed {
			return fmt.Errorf("no mapping found for %s in %s:%s", name, namespace, filePath)
		}
		sf.Refs = newRefs
		return vault.SaveSecretFile(sf)
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
		sf, err := vault.LoadSecretFile(oldName)
		if err != nil {
			return fmt.Errorf("secret not found: %s", oldName)
		}
		if _, err := vault.LoadSecretFile(newName); err == nil {
			return fmt.Errorf("secret already exists: %s", newName)
		}
		sf.Name = newName
		if err := vault.SaveSecretFile(sf); err != nil {
			return fmt.Errorf("failed to save new secret: %w", err)
		}
		if err := vault.DeleteSecretFile(oldName); err != nil {
			return fmt.Errorf("failed to delete old secret: %w", err)
		}
		idx, ok := state.Secrets[oldName]
		if ok {
			delete(state.Secrets, oldName)
			state.Secrets[newName] = idx
		}
		return nil
	}); err != nil {
		return err
	}

	fmt.Printf("Renamed secret: %s → %s\n", oldName, newName)
	audit.Log("secret rename", fmt.Sprintf("%s → %s", oldName, newName))
	return nil
}

func runSecretRotate(cmd *cobra.Command, args []string) error {
	defer mustLock()()
	name := args[0]
	cfg, vault, _, err := loadContext()
	if err != nil {
		return err
	}

	value, err := utils.PromptPassword("Enter new secret value:")
	if err != nil {
		return err
	}
	if value == "" {
		return fmt.Errorf("secret value cannot be empty")
	}

	reason, err := utils.PromptPassword("Rotation reason (optional):")
	if err != nil {
		return err
	}
	if reason == "" {
		reason = "Manual rotation"
	}

	encValue, err := vault.Encrypt([]byte(value))
	if err != nil {
		return fmt.Errorf("failed to encrypt secret: %w", err)
	}

	gitClient := git.NewClient(config.VaultPath())
	if err := vaultCommitPush(gitClient, vault, fmt.Sprintf("daemon-hound: rotate secret %s", name), func(state *models.VaultState) error {
		sf, err := vault.LoadSecretFile(name)
		if err != nil {
			return fmt.Errorf("secret not found: %s", name)
		}
		newVer := nextVersion(sf)
		sf.Versions[newVer] = models.SecretVersion{
			CreatedAt: time.Now(),
			Reason:    reason,
			Value:     encValue,
		}
		sf.Latest = newVer
		if err := vault.SaveSecretFile(sf); err != nil {
			return err
		}
		state.Secrets[name] = models.SecretIndex{
			Latest:    sf.Latest,
			Versions:  len(sf.Versions),
			UpdatedAt: time.Now(),
		}
		return nil
	}); err != nil {
		return err
	}

	// Reload to get the new latest version and update refs.
	sf, err := vault.LoadSecretFile(name)
	if err != nil {
		return fmt.Errorf("failed to reload secret: %w", err)
	}
	secretValue, err := getLatestValue(vault, sf)
	if err != nil {
		return fmt.Errorf("failed to decrypt secret: %w", err)
	}

	updatedCount := 0
	for _, ref := range sf.Refs {
		if cfg.IsFileIgnored(ref.Namespace, ref.File) {
			continue
		}
		if err := updateFileWithSecret(cfg, ref, string(secretValue)); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to update %s in %s: %v\n", ref.File, ref.Namespace, err)
			continue
		}
		fmt.Printf("  → Updated %s in %s  (%s)\n", ref.File, ref.Namespace, ref.Key)
		updatedCount++
	}

	fmt.Printf("Rotated secret: %s → %s (%s)\n", name, sf.Latest, reason)
	if updatedCount > 0 {
		fmt.Printf("%d file(s) marked dirty — run `dhd sync` to push\n", updatedCount)
	}
	audit.Log("secret rotate", fmt.Sprintf("%s %s refs=%d reason=%s", name, sf.Latest, updatedCount, reason))
	return nil
}

func runSecretRollback(cmd *cobra.Command, args []string) error {
	defer mustLock()()
	name := args[0]
	version := args[1]

	cfg, vault, _, err := loadContext()
	if err != nil {
		return err
	}

	gitClient := git.NewClient(config.VaultPath())
	if err := vaultCommitPush(gitClient, vault, fmt.Sprintf("daemon-hound: rollback secret %s to %s", name, version), func(state *models.VaultState) error {
		sf, err := vault.LoadSecretFile(name)
		if err != nil {
			return err
		}
		if _, ok := sf.Versions[version]; !ok {
			return fmt.Errorf("version %s not found for secret %s", version, name)
		}
		sf.Latest = version
		if err := vault.SaveSecretFile(sf); err != nil {
			return err
		}
		idx, ok := state.Secrets[name]
		if ok {
			idx.Latest = sf.Latest
			idx.UpdatedAt = time.Now()
			state.Secrets[name] = idx
		}
		return nil
	}); err != nil {
		return err
	}

	// Reload and update refs.
	sf, err := vault.LoadSecretFile(name)
	if err != nil {
		return fmt.Errorf("failed to reload secret: %w", err)
	}
	secretValue, err := getLatestValue(vault, sf)
	if err != nil {
		return fmt.Errorf("failed to decrypt secret: %w", err)
	}

	updatedCount := 0
	for _, ref := range sf.Refs {
		if cfg.IsFileIgnored(ref.Namespace, ref.File) {
			continue
		}
		if err := updateFileWithSecret(cfg, ref, string(secretValue)); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to update %s in %s: %v\n", ref.File, ref.Namespace, err)
			continue
		}
		fmt.Printf("  → Updated %s in %s  (%s)\n", ref.File, ref.Namespace, ref.Key)
		updatedCount++
	}

	fmt.Printf("Rolled back secret: %s → %s\n", name, version)
	if updatedCount > 0 {
		fmt.Printf("%d file(s) marked dirty — run `dhd sync` to push\n", updatedCount)
	}
	audit.Log("secret rollback", fmt.Sprintf("%s → %s refs=%d", name, version, updatedCount))
	return nil
}

func runSecretHistory(cmd *cobra.Command, args []string) error {
	name := args[0]
	_, vault, _, err := loadContext()
	if err != nil {
		return err
	}

	sf, err := vault.LoadSecretFile(name)
	if err != nil {
		return err
	}

	fmt.Printf("Secret: %s\n", sf.Name)
	fmt.Printf("Created: %s by %s\n", sf.CreatedAt.Format("2006-01-02 15:04"), sf.CreatedBy)
	fmt.Printf("Latest:  %s\n\n", sf.Latest)

	// Sort versions for display.
	var versions []string
	for v := range sf.Versions {
		versions = append(versions, v)
	}
	for i := 0; i < len(versions); i++ {
		for j := i + 1; j < len(versions); j++ {
			vi, vj := 0, 0
			fmt.Sscanf(versions[i], "v%d", &vi)
			fmt.Sscanf(versions[j], "v%d", &vj)
			if vi > vj {
				versions[i], versions[j] = versions[j], versions[i]
			}
		}
	}

	for _, v := range versions {
		ver := sf.Versions[v]
		marker := ""
		if v == sf.Latest {
			marker = "  ← latest"
		}
		fmt.Printf("  %-4s  %s  %-20s%s\n", v, ver.CreatedAt.Format("2006-01-02 15:04"), ver.Reason, marker)
	}
	return nil
}

func runSecretDiff(cmd *cobra.Command, args []string) error {
	name := args[0]
	verA := args[1]
	verB := args[2]

	_, vault, _, err := loadContext()
	if err != nil {
		return err
	}

	sf, err := vault.LoadSecretFile(name)
	if err != nil {
		return err
	}

	valA, err := getVersionValue(vault, sf, verA)
	if err != nil {
		return fmt.Errorf("version %s: %w", verA, err)
	}
	valB, err := getVersionValue(vault, sf, verB)
	if err != nil {
		return fmt.Errorf("version %s: %w", verB, err)
	}

	if bytes.Equal(valA, valB) {
		fmt.Println("Values are identical.")
		return nil
	}

	// Show a safe diff — don't print the actual values.
	fmt.Printf("Secret: %s\n", name)
	fmt.Printf("Version %s: %d bytes\n", verA, len(valA))
	fmt.Printf("Version %s: %d bytes\n", verB, len(valB))
	fmt.Println("Values differ (not shown for security).")
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

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

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/output"
	"github.com/spf13/cobra"
)

var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Manage Git integration for the vault repository",
	Long: `Configure Git so the vault repository is DaemonHound-aware.

Subcommands:
  setup   — Install all Git configuration (merge driver, diff driver,
            filters, hooks, .gitattributes, .gitignore, rerere)
  hook    — Install or update Git hooks (pre-commit, post-merge, etc.)

This replaces the need to manually run git config commands.`,
}

var gitSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure Git for DaemonHound vault repository",
	Long: `Installs all Git configuration needed for the vault:

  • Merge driver (merge.dhd) — semantic merge of encrypted files
  • Diff driver (diff.dhd) — show decrypted content in git diff
  • Clean/smudge filters (filter.dhd) — transparent encryption
  • .gitattributes — route *.age files through dhd drivers
  • .gitignore — ignore dhd internal directories
  • rerere — remember conflict resolutions
  • Hooks — pre-commit, post-merge, post-checkout, pre-push

Run this after dhd init or whenever you want to refresh Git setup.`,
	RunE: runGitSetup,
}

var gitHookCmd = &cobra.Command{
	Use:   "hook [name]",
	Short: "Install or update Git hooks",
	Long: `Install Git hooks into the vault repository.

Available hooks:
  pre-commit     — Validate staged files (JSON, YAML, env keys, etc.)
  post-merge     — Refresh metadata after pull/merge
  post-checkout  — Update working tree after branch switch
  pre-push       — Prevent pushing plaintext secrets

With no argument, installs all hooks.
With a name, installs only that hook.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runGitHook,
}

func init() {
	gitCmd.AddCommand(gitSetupCmd)
	gitCmd.AddCommand(gitHookCmd)
	rootCmd.AddCommand(gitCmd)
}

func runGitSetup(cmd *cobra.Command, args []string) error {
	cfg := config.NewConfig()
	if err := cfg.Load(); err != nil {
		return fmt.Errorf("not initialized — run `dhd init` first")
	}

	vaultPath := config.VaultPath()
	gc := git.NewClient(vaultPath)

	fmt.Println(output.Bold("Configuring Git for DaemonHound vault..."))
	fmt.Println()

	// 1. Merge driver
	if err := gc.SetMergeDriverName("dhd", "DaemonHound Merge Driver"); err != nil {
		fmt.Println(output.Fail(fmt.Sprintf("merge driver name: %v", err)))
	} else {
		fmt.Println(output.OK("Merge driver name configured"))
	}
	if err := gc.SetMergeDriver("dhd", "dhd merge %O %A %B %P"); err != nil {
		fmt.Println(output.Fail(fmt.Sprintf("merge driver: %v", err)))
	} else {
		fmt.Println(output.OK("Merge driver configured"))
	}

	// 2. Diff driver
	if err := gc.SetDiffDriverName("dhd", "DaemonHound Diff Driver"); err != nil {
		fmt.Println(output.Fail(fmt.Sprintf("diff driver name: %v", err)))
	} else {
		fmt.Println(output.OK("Diff driver name configured"))
	}
	if err := gc.SetDiffTextconv("dhd", "dhd diff --textconv"); err != nil {
		fmt.Println(output.Fail(fmt.Sprintf("diff textconv: %v", err)))
	} else {
		fmt.Println(output.OK("Diff textconv configured"))
	}

	// 3. Clean/smudge filter
	if err := gc.SetFilterName("dhd", "DaemonHound Filter"); err != nil {
		fmt.Println(output.Fail(fmt.Sprintf("filter name: %v", err)))
	} else {
		fmt.Println(output.OK("Filter name configured"))
	}
	if err := gc.SetFilterClean("dhd", "dhd encrypt"); err != nil {
		fmt.Println(output.Fail(fmt.Sprintf("filter clean: %v", err)))
	} else {
		fmt.Println(output.OK("Filter clean configured"))
	}
	if err := gc.SetFilterSmudge("dhd", "dhd decrypt"); err != nil {
		fmt.Println(output.Fail(fmt.Sprintf("filter smudge: %v", err)))
	} else {
		fmt.Println(output.OK("Filter smudge configured"))
	}

	// 4. .gitattributes
	if err := writeGitAttributes(gc); err != nil {
		fmt.Println(output.Fail(fmt.Sprintf(".gitattributes: %v", err)))
	} else {
		fmt.Println(output.OK(".gitattributes configured"))
	}

	// 5. .gitignore
	if err := writeGitIgnore(vaultPath); err != nil {
		fmt.Println(output.Fail(fmt.Sprintf(".gitignore: %v", err)))
	} else {
		fmt.Println(output.OK(".gitignore configured"))
	}

	// 6. rerere
	if err := gc.SetRerereEnabled(true); err != nil {
		fmt.Println(output.Fail(fmt.Sprintf("rerere: %v", err)))
	} else {
		fmt.Println(output.OK("rerere enabled"))
	}

	// 7. Hooks
	if err := installAllHooks(gc); err != nil {
		fmt.Println(output.Fail(fmt.Sprintf("hooks: %v", err)))
	} else {
		fmt.Println(output.OK("Hooks installed"))
	}

	// 8. Recommended settings (informational only)
	fmt.Println()
	fmt.Println(output.Yellow("Recommended Git settings (not changed automatically):"))
	fmt.Println("  core.fileMode=false     — avoid chmod noise")
	fmt.Println("  core.autocrlf=input     — consistent line endings")
	fmt.Println("  core.fsmonitor=true     — faster status in large vaults")

	fmt.Println()
	fmt.Println(output.Green("Git setup complete."))
	return nil
}

func runGitHook(cmd *cobra.Command, args []string) error {
	cfg := config.NewConfig()
	if err := cfg.Load(); err != nil {
		return fmt.Errorf("not initialized — run `dhd init` first")
	}

	vaultPath := config.VaultPath()
	gc := git.NewClient(vaultPath)

	if len(args) == 0 {
		if err := installAllHooks(gc); err != nil {
			return fmt.Errorf("install hooks: %w", err)
		}
		fmt.Println(output.Green("All hooks installed."))
		return nil
	}

	hookName := args[0]
	if err := installHook(gc, hookName); err != nil {
		return fmt.Errorf("install %s hook: %w", hookName, err)
	}
	fmt.Printf("%s hook installed.\n", hookName)
	return nil
}

// writeGitAttributes creates or updates .gitattributes in the vault.
func writeGitAttributes(gc *git.Client) error {
	attrs, err := gc.ReadGitAttributes()
	if err != nil {
		return err
	}

	entries := []struct {
		pattern string
		line    string
	}{
		{"state.toml.age", "state.toml.age merge=dhd diff=dhd\n"},
		{"secrets/*.toml.age", "secrets/*.toml.age merge=dhd diff=dhd\n"},
		{"*.env.age", "*.env.age filter=dhd diff=dhd merge=dhd\n"},
		{"*.json.age", "*.json.age filter=dhd diff=dhd merge=dhd\n"},
		{"*.csv.age", "*.csv.age filter=dhd diff=dhd merge=dhd\n"},
		{"*.age", "*.age diff=dhd\n"},
	}

	var missing []string
	for _, e := range entries {
		if !strings.Contains(attrs, e.pattern) {
			missing = append(missing, e.line)
		}
	}

	if len(missing) > 0 {
		content := strings.Join(missing, "")
		if err := gc.WriteGitAttributes(content); err != nil {
			return err
		}
	}
	return nil
}

// writeGitIgnore creates or updates .gitignore in the vault.
func writeGitIgnore(vaultPath string) error {
	path := filepath.Join(vaultPath, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	existing := string(data)

	entries := []string{
		".dhd/",
		".dhd-cache/",
		".dhd-temp/",
		".dhd-locks/",
	}

	var missing []string
	for _, e := range entries {
		if !strings.Contains(existing, e) {
			missing = append(missing, e)
		}
	}

	if len(missing) > 0 {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		for _, e := range missing {
			if _, err := f.WriteString(e + "\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

// installAllHooks installs all supported Git hooks.
func installAllHooks(gc *git.Client) error {
	hooks := []string{"pre-commit", "post-merge", "post-checkout", "pre-push"}
	for _, name := range hooks {
		if err := installHook(gc, name); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// installHook writes a hook script that delegates to dhd.
func installHook(gc *git.Client, name string) error {
	var script string
	switch name {
	case "pre-commit":
		script = preCommitHook()
	case "post-merge":
		script = postMergeHook()
	case "post-checkout":
		script = postCheckoutHook()
	case "pre-push":
		script = prePushHook()
	default:
		return fmt.Errorf("unknown hook: %s", name)
	}

	hooksDir := filepath.Join(config.VaultPath(), ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return err
	}

	path := filepath.Join(hooksDir, name)
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		return err
	}
	return nil
}

func preCommitHook() string {
	return `#!/bin/sh
# DaemonHound pre-commit hook
# Validates staged files before commit.

set -e

echo "[dhd] Running pre-commit checks..."

# Check for plaintext secrets in staged files
STAGED=$(git diff --cached --name-only)
for file in $STAGED; do
    # Reject unencrypted .env files (should be .env.age)
    case "$file" in
        *.env)
            if ! echo "$file" | grep -q '\.age$'; then
                echo "[dhd] ERROR: Unencrypted env file staged: $file"
                echo "[dhd] Use 'dhd track' to manage this file through the vault."
                exit 1
            fi
            ;;
    esac
done

# Validate JSON files
for file in $STAGED; do
    case "$file" in
        *.json)
            if command -v jq >/dev/null 2>&1; then
                if ! jq empty "$file" 2>/dev/null; then
                    echo "[dhd] ERROR: Invalid JSON: $file"
                    exit 1
                fi
            fi
            ;;
    esac
done

# Validate YAML files
for file in $STAGED; do
    case "$file" in
        *.yaml|*.yml)
            if command -v python3 >/dev/null 2>&1; then
                if ! python3 -c "import yaml; yaml.safe_load(open('$file'))" 2>/dev/null; then
                    echo "[dhd] ERROR: Invalid YAML: $file"
                    exit 1
                fi
            fi
            ;;
    esac
done

echo "[dhd] Pre-commit checks passed."
`
}

func postMergeHook() string {
	return `#!/bin/sh
# DaemonHound post-merge hook
# Refreshes metadata after pull or merge.

set -e

echo "[dhd] Post-merge refresh..."

# If dhd is available, trigger a sync to restore any pulled files
if command -v dhd >/dev/null 2>&1; then
    dhd sync --namespace "" 2>/dev/null || true
fi

echo "[dhd] Post-merge complete."
`
}

func postCheckoutHook() string {
	return `#!/bin/sh
# DaemonHound post-checkout hook
# Runs after branch checkout or worktree switch.
# Arguments: $1 = previous HEAD, $2 = new HEAD, $3 = flag (1=branch, 0=file)

set -e

echo "[dhd] Post-checkout refresh..."

# Only run on branch checkout (flag=1), not file checkout
if [ "$3" = "1" ]; then
    if command -v dhd >/dev/null 2>&1; then
        dhd sync --namespace "" 2>/dev/null || true
    fi
fi

echo "[dhd] Post-checkout complete."
`
}

func prePushHook() string {
	return `#!/bin/sh
# DaemonHound pre-push hook
# Prevents pushing plaintext secrets or invalid metadata.

set -e

echo "[dhd] Running pre-push checks..."

# Check for unencrypted sensitive files in the push
while read local_ref local_sha remote_ref remote_sha; do
    FILES=$(git diff --name-only "$remote_sha" "$local_sha" 2>/dev/null || true)
    for file in $FILES; do
        case "$file" in
            *.env|*.env.local|.envrc)
                if ! echo "$file" | grep -q '\.age$'; then
                    echo "[dhd] ERROR: Attempting to push unencrypted env file: $file"
                    exit 1
                fi
                ;;
            id_rsa|id_ed25519|*.pem|*.key)
                echo "[dhd] ERROR: Attempting to push private key: $file"
                exit 1
                ;;
        esac
    done
done

echo "[dhd] Pre-push checks passed."
`
}

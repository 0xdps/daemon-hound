package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/output"
	"github.com/spf13/cobra"
)

var doctorFix bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check DaemonHound setup and report any issues",
	Long: `Run a series of health checks on the local DaemonHound installation:

  - Config file present and parseable
  - Age identity key present
  - Vault git repository present
  - Git merge driver configured for vault files
  - Git diff driver configured for vault files
  - Git clean/smudge filter configured
  - .gitattributes routes *.age files through dhd drivers
  - .gitignore ignores dhd internal directories
  - rerere enabled for remembered conflict resolutions
  - Git hooks installed (pre-commit, post-merge, post-checkout, pre-push)
  - Git remote configured
  - Vault remote reachable (network)
  - Local namespace bindings pointing to existing directories
  - Pending push status

Use --fix to attempt automatic repair of common issues.`,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Attempt to auto-repair common issues")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	issues := 0

	// 1. Config
	cfg := config.NewConfig()
	if err := cfg.Load(); err != nil {
		fmt.Println(output.Fail("Config not found — run `dhd init` first"))
		fmt.Println("\n1 issue found.")
		return nil
	}
	fmt.Println(output.OK(fmt.Sprintf("Config loaded  (machine_id: %s)", cfg.MachineID())))

	// 2. Identity file
	if _, err := os.Stat(config.IdentityPath()); err != nil {
		fmt.Println(output.Fail(fmt.Sprintf("Identity file missing: %s", config.IdentityPath())))
		issues++
	} else {
		fmt.Println(output.OK(fmt.Sprintf("Identity file: %s", config.IdentityPath())))
	}

	// 3. Vault directory
	if _, err := os.Stat(config.VaultPath()); err != nil {
		fmt.Println(output.Fail(fmt.Sprintf("Vault directory missing: %s", config.VaultPath())))
		if doctorFix {
			if err := os.MkdirAll(config.VaultPath(), 0755); err == nil {
				fmt.Println(output.OK("  → Created vault directory"))
			}
		}
		issues++
	} else {
		fmt.Println(output.OK(fmt.Sprintf("Vault directory: %s", config.VaultPath())))
	}

	// 4. Vault git repo
	gitDir := filepath.Join(config.VaultPath(), ".git")
	if _, err := os.Stat(gitDir); err != nil {
		fmt.Println(output.Fail("Vault is not a git repository"))
		if doctorFix {
			if err := git.Init(config.VaultPath()); err == nil {
				fmt.Println(output.OK("  → Initialized git repository"))
				if remote := cfg.VaultRemote(); remote != "" {
					gc := git.NewClient(config.VaultPath())
					if err := gc.AddRemote(remote); err == nil {
						fmt.Println(output.OK(fmt.Sprintf("  → Added remote: %s", remote)))
					}
				}
			}
		}
		issues++
	} else {
		fmt.Println(output.OK("Vault is a git repository"))
	}

	// 5. Git merge driver configuration
	gc := git.NewClient(config.VaultPath())
	mergeDriver, _ := gc.GetMergeDriver("dhd")
	if mergeDriver == "" {
		fmt.Println(output.Warn("Git merge driver not configured for vault"))
		if doctorFix {
			if err := gc.SetMergeDriverName("dhd", "DaemonHound Merge Driver"); err == nil {
				fmt.Println(output.OK("  → Set merge.dhd.name"))
			}
			if err := gc.SetMergeDriver("dhd", "dhd merge %O %A %B %P"); err == nil {
				fmt.Println(output.OK("  → Set merge.dhd.driver"))
			}
		}
		issues++
	} else {
		fmt.Println(output.OK(fmt.Sprintf("Git merge driver: %s", mergeDriver)))
	}

	// 6. Git diff driver configuration
	diffDriver, _ := gc.GetDiffDriver("dhd")
	if diffDriver == "" {
		fmt.Println(output.Warn("Git diff driver not configured for vault"))
		if doctorFix {
			if err := gc.SetDiffDriverName("dhd", "DaemonHound Diff Driver"); err == nil {
				fmt.Println(output.OK("  → Set diff.dhd.name"))
			}
			if err := gc.SetDiffTextconv("dhd", "dhd diff --textconv"); err == nil {
				fmt.Println(output.OK("  → Set diff.dhd.textconv"))
			}
		}
		issues++
	} else {
		fmt.Println(output.OK(fmt.Sprintf("Git diff driver: %s", diffDriver)))
	}

	// 7. Git clean/smudge filter configuration
	filterClean, _ := gc.GetFilterClean("dhd")
	if filterClean == "" {
		fmt.Println(output.Warn("Git clean/smudge filter not configured"))
		if doctorFix {
			if err := gc.SetFilterName("dhd", "DaemonHound Filter"); err == nil {
				fmt.Println(output.OK("  → Set filter.dhd.name"))
			}
			if err := gc.SetFilterClean("dhd", "dhd encrypt"); err == nil {
				fmt.Println(output.OK("  → Set filter.dhd.clean"))
			}
			if err := gc.SetFilterSmudge("dhd", "dhd decrypt"); err == nil {
				fmt.Println(output.OK("  → Set filter.dhd.smudge"))
			}
		}
		issues++
	} else {
		fmt.Println(output.OK(fmt.Sprintf("Git filter: clean=%s", filterClean)))
	}

	// 8. .gitattributes for encrypted vault files
	attrs, _ := gc.ReadGitAttributes()
	hasStateAttr := strings.Contains(attrs, "state.toml.age")
	hasSecretsAttr := strings.Contains(attrs, "secrets/*.toml.age")
	if !hasStateAttr || !hasSecretsAttr {
		fmt.Println(output.Warn(".gitattributes missing merge driver entries for vault files"))
		if doctorFix {
			var sb strings.Builder
			if !hasStateAttr {
				sb.WriteString("state.toml.age merge=dhd diff=dhd\n")
			}
			if !hasSecretsAttr {
				sb.WriteString("secrets/*.toml.age merge=dhd diff=dhd\n")
			}
			if err := gc.WriteGitAttributes(sb.String()); err == nil {
				fmt.Println(output.OK("  → Added .gitattributes entries"))
			}
		}
		issues++
	} else {
		fmt.Println(output.OK(".gitattributes configured for vault drivers"))
	}

	// 9. .gitignore for dhd internal directories
	gitignorePath := filepath.Join(config.VaultPath(), ".gitignore")
	gitignoreData, _ := os.ReadFile(gitignorePath)
	gitignore := string(gitignoreData)
	if !strings.Contains(gitignore, ".dhd/") {
		fmt.Println(output.Warn(".gitignore missing dhd internal directories"))
		if doctorFix {
			f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				f.WriteString(".dhd/\n.dhd-cache/\n.dhd-temp/\n.dhd-locks/\n")
				f.Close()
				fmt.Println(output.OK("  → Added .gitignore entries"))
			}
		}
		issues++
	} else {
		fmt.Println(output.OK(".gitignore configured for dhd internals"))
	}

	// 10. rerere enabled
	rerereEnabled, _ := gc.GetRerereEnabled()
	if !rerereEnabled {
		fmt.Println(output.Warn("Git rerere not enabled"))
		if doctorFix {
			if err := gc.SetRerereEnabled(true); err == nil {
				fmt.Println(output.OK("  → Enabled rerere"))
			}
		}
		issues++
	} else {
		fmt.Println(output.OK("Git rerere enabled"))
	}

	// 11. Git hooks
	hooksDir := filepath.Join(config.VaultPath(), ".git", "hooks")
	hooks := []string{"pre-commit", "post-merge", "post-checkout", "pre-push"}
	missingHooks := 0
	for _, h := range hooks {
		if _, err := os.Stat(filepath.Join(hooksDir, h)); err != nil {
			missingHooks++
		}
	}
	if missingHooks > 0 {
		fmt.Println(output.Warn(fmt.Sprintf("Git hooks: %d/%d missing", missingHooks, len(hooks))))
		if doctorFix {
			if err := installAllHooks(gc); err == nil {
				fmt.Println(output.OK("  → Installed all hooks"))
			}
		}
		issues++
	} else {
		fmt.Println(output.OK("Git hooks installed"))
	}

	// 12. Git remote URL
	getRemote := exec.Command("git", "-C", config.VaultPath(), "remote", "get-url", "origin")
	if remoteURL, err := getRemote.Output(); err != nil {
		fmt.Println(output.Warn("No git remote configured"))
		if doctorFix {
			if remote := cfg.VaultRemote(); remote != "" {
				if err := gc.AddRemote(remote); err == nil {
					fmt.Println(output.OK(fmt.Sprintf("  → Added remote: %s", remote)))
				}
			}
		}
		issues++
	} else {
		fmt.Println(output.OK(fmt.Sprintf("Git remote: %s", string(remoteURL)[:len(remoteURL)-1]))) // trim newline
	}

	// 13. Remote connectivity
	lsRemote := exec.Command("git", "-C", config.VaultPath(), "ls-remote", "--exit-code", "origin")
	if err := lsRemote.Run(); err != nil {
		fmt.Println(output.Warn("Cannot reach vault remote (offline or misconfigured?)"))
	} else {
		fmt.Println(output.OK("Vault remote reachable"))
	}

	// 14. Namespace bindings
	bindings := cfg.Bindings()
	if len(bindings) == 0 {
		fmt.Println(output.OK("No namespace bindings (normal on a fresh machine)"))
	} else {
		for ns, path := range bindings {
			if _, err := os.Stat(path); err != nil {
				fmt.Println(output.Warn(fmt.Sprintf("Binding %s → %s  (directory not found)", ns, path)))
			} else {
				fmt.Println(output.OK(fmt.Sprintf("Binding %s → %s", ns, path)))
			}
		}
	}

	// 15. Pending push
	if cfg.PendingPush() {
		fmt.Println(output.Warn("Pending push: local vault commits have not yet been pushed to remote"))
		issues++
	}

	fmt.Println()
	if issues == 0 {
		fmt.Println(output.Green("All checks passed."))
	} else {
		fmt.Printf("%d issue(s) found.\n", issues)
	}
	return nil
}

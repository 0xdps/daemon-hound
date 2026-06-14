package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
		fmt.Println(output.Fail("Config not found — run `dh init` first"))
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

	// 5. Remote connectivity
	lsRemote := exec.Command("git", "-C", config.VaultPath(), "ls-remote", "--exit-code", "origin")
	if err := lsRemote.Run(); err != nil {
		fmt.Println(output.Warn("Cannot reach vault remote (offline or misconfigured?)"))
	} else {
		fmt.Println(output.OK("Vault remote reachable"))
	}

	// 6. Namespace bindings
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

	// 7. Pending push
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

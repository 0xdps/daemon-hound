package cmd

import (
	"fmt"

	"github.com/0xdps/daemon-hound/internal/audit"
	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/output"
	"github.com/0xdps/daemon-hound/internal/sync"
	"github.com/spf13/cobra"
)

var syncDryRun bool
var syncNamespace string

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync tracked files with the vault",
	Long: `Pull remote changes from the vault, then push local dirty files.

Use --dry-run to preview what would be synced without making changes.
Use --namespace to sync only a specific project.`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "Preview what would be synced without making any changes")
	syncCmd.Flags().StringVar(&syncNamespace, "namespace", "", "Only sync files for this namespace (e.g. github.com/you/repo)")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	if !syncDryRun {
		defer mustLock()()
	}

	cfg, vault, tr, err := loadContext()
	if err != nil {
		return err
	}

	gitClient := git.NewClient(config.VaultPath())
	syncer := sync.NewSyncer(vault, tr, gitClient, cfg)
	if syncNamespace != "" {
		syncer = syncer.WithNamespaceFilter(syncNamespace)
	}

	if syncDryRun {
		results, err := syncer.DryRun()
		if err != nil {
			return err
		}
		if len(results) == 0 {
			fmt.Println("Everything is up to date.")
			return nil
		}
		fmt.Println("Dry run — no changes will be made:")
		for _, r := range results {
			fmt.Printf("  → %-40s  %-20s  %s\n", r.File.Namespace, r.File.RelPath, output.Action(r.Action))
		}
		return nil
	}

	results, err := syncer.Sync()
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Println("Everything is up to date.")
		if cfg.PendingPush() {
			fmt.Println(output.Yellow("Note: a previous push is still pending — run `dhd sync` when online."))
		}
		return nil
	}

	pushed, pulled, errors := 0, 0, 0
	for _, r := range results {
		if r.Error != nil {
			fmt.Printf("  → %-40s  %-20s  %s\n", r.File.Namespace, r.File.RelPath, output.Red("error: "+r.Error.Error()))
			errors++
		} else {
			fmt.Printf("  → %-40s  %-20s  %s\n", r.File.Namespace, r.File.RelPath, output.Action(r.Action))
			switch r.Action {
			case "pushed":
				pushed++
			case "pulled":
				pulled++
			}
		}
	}

	fmt.Printf("\n%s (%d pushed, %d pulled", output.Bold("Done"), pushed, pulled)
	if errors > 0 {
		fmt.Printf(", %s", output.Red(fmt.Sprintf("%d error(s)", errors)))
	}
	fmt.Println(")")

	if cfg.PendingPush() {
		fmt.Println(output.Yellow("Note: push to remote is still pending — run `dhd sync` when online."))
	}

	audit.Log("sync", fmt.Sprintf("pushed:%d pulled:%d errors:%d", pushed, pulled, errors))
	return nil
}

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/0xdps/daemon-hound/internal/conflicts"
	"github.com/spf13/cobra"
)

var conflictsCmd = &cobra.Command{
	Use:   "conflicts",
	Short: "Manage vault merge conflicts",
	Long:  "Review and resolve merge conflicts detected by the daemon",
}

var conflictsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all detected conflicts",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := conflicts.NewStore()
		if err != nil {
			return fmt.Errorf("failed to open conflicts store: %w", err)
		}

		all, err := store.List()
		if err != nil {
			return fmt.Errorf("failed to list conflicts: %w", err)
		}

		if len(all) == 0 {
			fmt.Println("No conflicts recorded.")
			return nil
		}

		fmt.Printf("Total conflicts: %d\n\n", len(all))

		for i, c := range all {
			status := "⚠️  PENDING"
			if c.ResolvedAt != nil {
				status = fmt.Sprintf("✓ RESOLVED (%s)", c.ResolutionStrategy)
			}

			fmt.Printf("[%d] %s\n", i+1, c.FilePath)
			fmt.Printf("    Status: %s\n", status)
			fmt.Printf("    Detected: %s\n", c.DetectedAt.Format("2006-01-02 15:04:05"))

			if c.ResolvedAt != nil {
				fmt.Printf("    Resolved: %s\n", c.ResolvedAt.Format("2006-01-02 15:04:05"))
			}

			fmt.Printf("    Local:  %s\n", c.LocalHash[:12])
			fmt.Printf("    Remote: %s\n\n", c.RemoteHash[:12])
		}

		return nil
	},
}

var conflictsShowCmd = &cobra.Command{
	Use:   "show <file>",
	Short: "Show details of a specific conflict",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := conflicts.NewStore()
		if err != nil {
			return fmt.Errorf("failed to open conflicts store: %w", err)
		}

		filePath := args[0]
		conflict, err := store.Get(filePath)
		if err != nil {
			return fmt.Errorf("conflict not found: %w", err)
		}

		fmt.Printf("Conflict: %s\n", conflict.FilePath)
		fmt.Printf("Detected: %s\n\n", conflict.DetectedAt.Format("2006-01-02 15:04:05"))

		if conflict.ResolvedAt != nil {
			fmt.Printf("Status: ✓ RESOLVED with '%s' strategy\n", conflict.ResolutionStrategy)
			fmt.Printf("Resolved: %s\n\n", conflict.ResolvedAt.Format("2006-01-02 15:04:05"))
		} else {
			fmt.Println("Status: ⚠️  PENDING RESOLUTION")
			fmt.Println()

			fmt.Println("LOCAL VERSION (this machine):")
			fmt.Printf("Hash: %s\n", conflict.LocalHash)
			if conflict.LocalContent != "" {
				fmt.Printf("Content:\n%s\n\n", conflict.LocalContent)
			}

			fmt.Println("REMOTE VERSION (other machine):")
			fmt.Printf("Hash: %s\n", conflict.RemoteHash)
			if conflict.RemoteContent != "" {
				fmt.Printf("Content:\n%s\n\n", conflict.RemoteContent)
			}

			fmt.Println("To resolve this conflict, run:")
			fmt.Printf("  dhd conflicts resolve %s --strategy local   # Keep local changes\n", filePath)
			fmt.Printf("  dhd conflicts resolve %s --strategy remote  # Use remote changes\n", filePath)
		}

		return nil
	},
}

var conflictsResolveCmd = &cobra.Command{
	Use:   "resolve <file>",
	Short: "Resolve a conflict with chosen strategy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		strategy, err := cmd.Flags().GetString("strategy")
		if err != nil {
			return err
		}

		if strategy != "local" && strategy != "remote" {
			return fmt.Errorf("invalid strategy: %s (must be 'local' or 'remote')", strategy)
		}

		store, err := conflicts.NewStore()
		if err != nil {
			return fmt.Errorf("failed to open conflicts store: %w", err)
		}

		filePath := args[0]
		conflict, err := store.Get(filePath)
		if err != nil {
			return fmt.Errorf("conflict not found: %w", err)
		}

		if conflict.ResolvedAt != nil {
			fmt.Printf("Conflict already resolved with '%s' strategy on %s\n",
				conflict.ResolutionStrategy, conflict.ResolvedAt.Format("2006-01-02 15:04:05"))
			return nil
		}

		// Mark as resolved
		now := time.Now()
		conflict.ResolvedAt = &now
		conflict.ResolutionStrategy = strategy

		if err := store.Add(conflict); err != nil {
			return fmt.Errorf("failed to save resolution: %w", err)
		}

		fmt.Printf("✓ Conflict resolved with '%s' strategy\n", strategy)

		// Note: Actual git resolution would happen in daemon next sync
		fmt.Println("\nNote: The daemon will apply this resolution on the next sync cycle.")
		fmt.Println("To re-sync immediately, run: dhd sync")

		return nil
	},
}

var conflictsClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all resolved conflicts",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := conflicts.NewStore()
		if err != nil {
			return fmt.Errorf("failed to open conflicts store: %w", err)
		}

		all, err := store.List()
		if err != nil {
			return fmt.Errorf("failed to list conflicts: %w", err)
		}

		var pending []*conflicts.Conflict
		for _, c := range all {
			if c.ResolvedAt == nil {
				pending = append(pending, c)
			}
		}

		if len(pending) > 0 {
			fmt.Printf("⚠️  Cannot clear: %d conflicts still pending\n", len(pending))
			fmt.Println("\nPending conflicts:")
			for _, c := range pending {
				fmt.Printf("  - %s\n", c.FilePath)
			}
			return nil
		}

		// Clear conflicts file
		if err := os.Remove(os.ExpandEnv("$HOME/.dh/conflicts.json")); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to clear conflicts: %w", err)
		}

		fmt.Println("✓ All resolved conflicts cleared")
		return nil
	},
}

func init() {
	conflictsResolveCmd.Flags().StringP("strategy", "s", "local", "Resolution strategy: local or remote")
	conflictsCmd.AddCommand(conflictsListCmd)
	conflictsCmd.AddCommand(conflictsShowCmd)
	conflictsCmd.AddCommand(conflictsResolveCmd)
	conflictsCmd.AddCommand(conflictsClearCmd)
	rootCmd.AddCommand(conflictsCmd)
}

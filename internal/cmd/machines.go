package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/output"
	"github.com/spf13/cobra"
)

var machinesOutput string

var machinesCmd = &cobra.Command{
	Use:   "machines",
	Short: "List machines that have backup files in the vault",
	Long: `List all machine UUIDs that have stored backup files in the vault.

Each machine that has run 'dh track --mode backup' appears here. The current
machine is marked with '(this machine)'.`,
	RunE: runMachines,
}

func init() {
	machinesCmd.Flags().StringVar(&machinesOutput, "output", "table", "Output format: table or json")
	rootCmd.AddCommand(machinesCmd)
}

func runMachines(cmd *cobra.Command, args []string) error {
	cfg := config.NewConfig()
	_ = cfg.Load()

	backupDir := filepath.Join(config.VaultPath(), "backup")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No backup machines found in vault.")
			return nil
		}
		return fmt.Errorf("failed to read vault backup directory: %w", err)
	}

	type machineEntry struct {
		ID        string `json:"machine_id"`
		IsCurrent bool   `json:"is_current"`
	}
	var machines []machineEntry
	for _, e := range entries {
		if e.IsDir() {
			machines = append(machines, machineEntry{
				ID:        e.Name(),
				IsCurrent: e.Name() == cfg.MachineID(),
			})
		}
	}

	if len(machines) == 0 {
		fmt.Println("No backup machines found in vault.")
		return nil
	}

	if machinesOutput == "json" {
		data, _ := json.MarshalIndent(machines, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("%-40s  %s\n", output.Bold("Machine ID"), output.Bold("Note"))
	fmt.Printf("%-40s  %s\n", "----------", "----")
	for _, m := range machines {
		note := ""
		if m.IsCurrent {
			note = output.Green("(this machine)")
		}
		fmt.Printf("%-40s  %s\n", m.ID, note)
	}
	return nil
}


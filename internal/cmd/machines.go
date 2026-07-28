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

Each machine that has run 'dhd track --mode backup' appears here. The current
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

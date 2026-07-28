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

	"github.com/0xdps/daemon-hound/internal/keychain"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove the stored master password from the OS keychain",
	Long: `Remove the cached master password from your OS keychain.

You will be prompted for your master password again on the next command.`,
	RunE: runLogout,
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}

func runLogout(cmd *cobra.Command, args []string) error {
	if err := keychain.Delete(); err != nil {
		// Treat "not found" as already logged out rather than an error.
		if keychain.IsSet() {
			return fmt.Errorf("failed to remove password from keychain: %w", err)
		}
		fmt.Println("Already logged out (no password stored in keychain).")
		return nil
	}
	fmt.Println("Master password removed from keychain.")
	return nil
}

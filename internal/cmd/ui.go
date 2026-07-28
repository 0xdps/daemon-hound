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
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/0xdps/daemon-hound/internal/web"
	"github.com/spf13/cobra"
)

var uiPort int
var uiNoBrowser bool

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Open the DaemonHound web UI",
	Long: `Start the DaemonHound web UI server and open it in your browser.

The UI provides:
  • Conflict resolver — 3-panel diff view (local | merged | remote)
  • Files browser    — view decrypted tracked files
  • Secrets manager  — view, rotate, and manage secrets
  • Sync status      — live daemon log stream
  • Settings         — manage tracked files

Protected by your master password. Session expires after 12 hours.`,
	RunE: runUI,
}

func init() {
	uiCmd.Flags().IntVar(&uiPort, "port", 0, "Port to listen on (default: auto 7734-7800)")
	uiCmd.Flags().BoolVar(&uiNoBrowser, "no-browser", false, "Don't open the browser automatically")
	rootCmd.AddCommand(uiCmd)
}

func runUI(cmd *cobra.Command, args []string) error {
	vault, err := loadVault()
	if err != nil {
		return fmt.Errorf("load vault: %w", err)
	}

	srv, err := web.NewServer(vault)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	url, err := srv.ListenAndServe(ctx)
	if err != nil {
		return fmt.Errorf("start server: %w", err)
	}

	fmt.Printf("DaemonHound UI running at: %s\n", url)
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	if !uiNoBrowser {
		// Give the server a moment to fully start before opening the browser.
		time.Sleep(100 * time.Millisecond)
		if err := openBrowser(url); err != nil {
			fmt.Fprintf(os.Stderr, "Note: could not open browser automatically: %v\n", err)
			fmt.Printf("Open manually: %s\n", url)
		}
	}

	// Block until Ctrl+C
	<-ctx.Done()
	return nil
}

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	return exec.Command(cmd, args...).Start()
}

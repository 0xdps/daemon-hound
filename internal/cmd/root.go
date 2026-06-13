package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// These are set at build time via ldflags
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:     "dh",
	Short:   "DaemonHound",
	Long:    `Daemon Hound is a tool for tracking and syncing data from various sources. It provides a simple CLI interface for managing your data.`,
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

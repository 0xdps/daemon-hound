package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "dh",
	Short: "DaemonHound",
	Long:  `Daemon Hound is a tool for tracking and syncing data from various sources. It provides a simple CLI interface for managing your data.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

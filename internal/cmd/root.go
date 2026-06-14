package cmd

import (
	"fmt"
	"os"

	"github.com/0xdps/daemon-hound/internal/config"
	"github.com/0xdps/daemon-hound/internal/lock"
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
	Long:    `Opinionated local config and secret management for developers.`,
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(rootCmd.Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// mustLock acquires an exclusive process lock for mutating commands.
// Returns a release function that must be deferred by the caller.
// Exits the process if the lock cannot be acquired.
func mustLock() func() {
	l := lock.New(config.AppDir())
	if err := l.Acquire(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return l.Release
}


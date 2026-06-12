package cmd

import (
	"fmt"

	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of tracked files",
	Long:  `Display the current status of all tracked files: clean, dirty, new, or missing.`,
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	_, vault, tr, err := loadContext()
	if err != nil {
		return err
	}

	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load vault state: %w", err)
	}

	if len(state.Files) == 0 {
		fmt.Println("No tracked files.")
		return nil
	}

	// Group by namespace
	byNS := make(map[string][]struct {
		file   models.TrackedFile
		status models.DirtyStatus
	})
	for _, file := range state.Files {
		status, err := tr.Status(file)
		if err != nil {
			status = models.DirtyStatus("error")
		}
		byNS[file.Namespace] = append(byNS[file.Namespace], struct {
			file   models.TrackedFile
			status models.DirtyStatus
		}{file, status})
	}

	for ns, files := range byNS {
		fmt.Printf("%s\n", ns)
		for _, f := range files {
			fmt.Printf("  %-20s  %s\n", f.file.RelPath, f.status)
		}
	}
	return nil
}

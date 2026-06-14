package cmd

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/0xdps/daemon-hound/internal/output"
	"github.com/spf13/cobra"
)

var statusNamespace string
var statusOutput string

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of tracked files",
	Long:  `Display the current status of all tracked files: clean, dirty, new, or missing.`,
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().StringVar(&statusNamespace, "namespace", "", "Filter to a specific namespace")
	statusCmd.Flags().StringVar(&statusOutput, "output", "table", "Output format: table or json")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, vault, tr, err := loadContext()
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

	type fileEntry struct {
		file   models.TrackedFile
		status models.DirtyStatus
	}

	byNS := make(map[string][]fileEntry)
	for _, file := range state.Files {
		if statusNamespace != "" && file.Namespace != statusNamespace {
			continue
		}
		fileStatus, err := tr.Status(file)
		if err != nil {
			fileStatus = models.DirtyStatus("error")
		}
		byNS[file.Namespace] = append(byNS[file.Namespace], fileEntry{file, fileStatus})
	}

	if statusOutput == "json" {
		type jsonFile struct {
			Namespace string `json:"namespace"`
			Path      string `json:"path"`
			Status    string `json:"status"`
			Mode      string `json:"mode"`
		}
		type jsonOut struct {
			Files       []jsonFile `json:"files"`
			PendingPush bool       `json:"pending_push"`
		}
		out := jsonOut{PendingPush: cfg.PendingPush()}
		for _, entries := range byNS {
			for _, e := range entries {
				out.Files = append(out.Files, jsonFile{
					Namespace: e.file.Namespace,
					Path:      e.file.RelPath,
					Status:    string(e.status),
					Mode:      string(e.file.Mode),
				})
			}
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Table output
	namespaces := make([]string, 0, len(byNS))
	for ns := range byNS {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)

	for _, ns := range namespaces {
		fmt.Printf("%s\n", output.Bold(ns))
		for _, f := range byNS[ns] {
			annotation := ""
			if f.file.Mode == models.ModeBackup {
				annotation = output.Dim("  (backup, this machine only)")
			}
			fmt.Printf("  %-20s  %s%s\n", f.file.RelPath, output.Status(string(f.status)), annotation)
		}
	}

	if cfg.PendingPush() {
		fmt.Println()
		fmt.Println(output.Yellow("Note: local vault commits are pending push to remote. Run `dh sync` when online."))
	}
	return nil
}


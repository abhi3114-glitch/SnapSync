package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/snapsync/snapsync/internal/snapshot"
	"github.com/snapsync/snapsync/internal/store"
	"github.com/snapsync/snapsync/pkg/models"
	"github.com/spf13/cobra"
)

func statusCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show repository status",
		Long:  "Displays information about the repository including statistics and health.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				return fmt.Errorf("repository path required (use --repo)")
			}

			return showStatus(repoPath, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}

func showStatus(repoPath string, jsonOutput bool) error {
	// Load repository info
	infoPath := filepath.Join(repoPath, "repo.json")
	data, err := os.ReadFile(infoPath)
	if err != nil {
		return fmt.Errorf("repository not found or invalid: %w", err)
	}

	var repoInfo models.RepositoryInfo
	if err := json.Unmarshal(data, &repoInfo); err != nil {
		return fmt.Errorf("invalid repository info: %w", err)
	}

	// Get storage stats
	cas, err := store.NewCAS(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}

	objectCount, totalSize, err := cas.Stats()
	if err != nil {
		return fmt.Errorf("failed to get storage stats: %w", err)
	}

	// Get snapshot count
	mgr, err := snapshot.NewManager(repoPath, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	snapshots, err := mgr.List()
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	status := struct {
		Version        int    `json:"version"`
		Encrypted      bool   `json:"encrypted"`
		SnapshotCount  int    `json:"snapshot_count"`
		ObjectCount    int    `json:"object_count"`
		TotalSize      int64  `json:"total_size"`
		TotalSizeHuman string `json:"total_size_human"`
	}{
		Version:        repoInfo.Version,
		Encrypted:      repoInfo.Encrypted,
		SnapshotCount:  len(snapshots),
		ObjectCount:    objectCount,
		TotalSize:      totalSize,
		TotalSizeHuman: formatBytes(totalSize),
	}

	if jsonOutput {
		output, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(output))
		return nil
	}

	// Pretty print
	fmt.Println("SnapSync Repository Status")
	fmt.Println("==========================")
	fmt.Printf("Path:       %s\n", repoPath)
	fmt.Printf("Version:    %d\n", status.Version)
	fmt.Printf("Encrypted:  %v\n", status.Encrypted)
	fmt.Printf("Snapshots:  %d\n", status.SnapshotCount)
	fmt.Printf("Objects:    %d\n", status.ObjectCount)
	fmt.Printf("Total Size: %s\n", status.TotalSizeHuman)

	if len(snapshots) > 0 {
		fmt.Println()
		fmt.Println("Latest snapshot:")
		latest := snapshots[0]
		fmt.Printf("  ID:      %s\n", latest.ID[:16]+"...")
		fmt.Printf("  Created: %s\n", latest.Timestamp.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Files:   %d\n", latest.Tree.FileCount)
	}

	return nil
}

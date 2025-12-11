package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/snapsync/snapsync/internal/snapshot"
	"github.com/spf13/cobra"
)

func listCmd() *cobra.Command {
	var (
		showTree  bool
		showFiles bool
		pattern   string
	)

	cmd := &cobra.Command{
		Use:   "list [snapshot-id]",
		Short: "List snapshots or files in a snapshot",
		Long:  "Lists all snapshots in the repository, or files in a specific snapshot.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				return fmt.Errorf("repository path required (use --repo)")
			}

			if len(args) > 0 {
				return listSnapshotContents(repoPath, args[0], showTree, showFiles, pattern)
			}

			return listSnapshots(repoPath)
		},
	}

	cmd.Flags().BoolVarP(&showTree, "tree", "t", false, "Show file tree for snapshot")
	cmd.Flags().BoolVarP(&showFiles, "files", "f", false, "Show all files in snapshot")
	cmd.Flags().StringVarP(&pattern, "pattern", "p", "", "Filter files by pattern")

	return cmd
}

func listSnapshots(repoPath string) error {
	mgr, err := snapshot.NewManager(repoPath, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	snapshots, err := mgr.List()
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		fmt.Println("No snapshots found")
		return nil
	}

	fmt.Printf("%-20s  %-20s  %10s  %10s  %s\n",
		"ID", "TIMESTAMP", "FILES", "SIZE", "DESCRIPTION")
	fmt.Println("------------------------------------------------------------------------------------")

	for _, snap := range snapshots {
		desc := snap.Description
		if len(desc) > 30 {
			desc = desc[:27] + "..."
		}

		fmt.Printf("%-20s  %-20s  %10d  %10s  %s\n",
			snap.ID[:16]+"...",
			snap.Timestamp.Format("2006-01-02 15:04:05"),
			snap.Tree.FileCount,
			formatBytes(snap.Stats.TotalSize),
			desc,
		)
	}

	fmt.Printf("\nTotal: %d snapshots\n", len(snapshots))
	return nil
}

func listSnapshotContents(repoPath, snapshotID string, showTree, showFiles bool, pattern string) error {
	mgr, err := snapshot.NewManager(repoPath, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	snap, err := mgr.Get(snapshotID)
	if err != nil {
		// Try prefix match
		snapshots, _ := mgr.List()
		for _, s := range snapshots {
			if len(snapshotID) >= 8 && len(s.ID) >= 8 && s.ID[:8] == snapshotID[:min(8, len(snapshotID))] {
				snap = s
				break
			}
		}
		if snap == nil {
			return fmt.Errorf("snapshot not found: %s", snapshotID)
		}
	}

	// Print snapshot info
	fmt.Printf("Snapshot: %s\n", snap.ID)
	fmt.Printf("Created:  %s\n", snap.Timestamp.Format(time.RFC3339))
	if snap.Description != "" {
		fmt.Printf("Desc:     %s\n", snap.Description)
	}
	fmt.Println()
	fmt.Printf("Files:    %d\n", snap.Tree.FileCount)
	fmt.Printf("Dirs:     %d\n", snap.Tree.DirCount)
	fmt.Printf("Size:     %s\n", formatBytes(snap.Stats.TotalSize))
	fmt.Printf("Stored:   %s\n", formatBytes(snap.Stats.StoredSize))
	fmt.Println()

	if showFiles || showTree {
		// Collect and sort paths
		var paths []string
		for path, node := range snap.Tree.Files {
			if node.IsDir {
				continue
			}
			paths = append(paths, path)
		}
		sort.Strings(paths)

		// Filter by pattern
		if pattern != "" {
			var filtered []string
			for _, p := range paths {
				// Simple contains match for now
				if matchesPattern(p, pattern) {
					filtered = append(filtered, p)
				}
			}
			paths = filtered
		}

		fmt.Printf("Files (%d):\n", len(paths))
		for _, path := range paths {
			node := snap.Tree.Files[path]
			fmt.Printf("  %10s  %s\n", formatBytes(node.Size), path)
		}
	}

	return nil
}

func matchesPattern(path, pattern string) bool {
	// Simple substring match for now
	return len(pattern) == 0 ||
		contains(path, pattern, true)
}

func contains(s, substr string, caseInsensitive bool) bool {
	if caseInsensitive {
		s = toLower(s)
		substr = toLower(substr)
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

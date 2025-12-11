package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "1.0.0"

	// Global flags
	repoPath   string
	configPath string
	verbose    bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "snapsync",
		Short: "SnapSync — Snapshot-Based Backup System",
		Long: `SnapSync is a high-performance backup tool with:
  • Content-defined chunking for efficient deduplication
  • AES-256-GCM encryption with Argon2id key derivation
  • ZSTD compression
  • S3-compatible cloud storage
  • Point-in-time recovery`,
		Version: version,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&repoPath, "repo", "r", "", "Repository path")
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Config file path")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")

	// Add commands
	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(backupCmd())
	rootCmd.AddCommand(restoreCmd())
	rootCmd.AddCommand(listCmd())
	rootCmd.AddCommand(statusCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

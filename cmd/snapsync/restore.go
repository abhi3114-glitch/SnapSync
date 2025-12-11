package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/snapsync/snapsync/internal/compress"
	"github.com/snapsync/snapsync/internal/config"
	"github.com/snapsync/snapsync/internal/crypto"
	"github.com/snapsync/snapsync/internal/restore"
	"github.com/snapsync/snapsync/internal/snapshot"
	"github.com/snapsync/snapsync/internal/store"
	"github.com/snapsync/snapsync/pkg/models"
	"github.com/spf13/cobra"
)

func restoreCmd() *cobra.Command {
	var (
		include      []string
		exclude      []string
		overwrite    bool
		dryRun       bool
		preservePerm bool
	)

	cmd := &cobra.Command{
		Use:   "restore [snapshot-id] [target]",
		Short: "Restore files from a snapshot",
		Long:  "Restores files from a snapshot to the target directory.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			snapshotID := args[0]
			targetPath := "."
			if len(args) > 1 {
				targetPath = args[1]
			}

			if repoPath == "" {
				return fmt.Errorf("repository path required (use --repo)")
			}

			opts := models.RestoreOptions{
				SnapshotID:     snapshotID,
				TargetPath:     targetPath,
				IncludePattern: include,
				ExcludePattern: exclude,
				Overwrite:      overwrite,
				PreservePerms:  preservePerm,
				DryRun:         dryRun,
			}

			return runRestore(repoPath, opts)
		},
	}

	cmd.Flags().StringArrayVarP(&include, "include", "i", nil, "Include patterns (glob)")
	cmd.Flags().StringArrayVarP(&exclude, "exclude", "x", nil, "Exclude patterns (glob)")
	cmd.Flags().BoolVarP(&overwrite, "overwrite", "f", false, "Overwrite existing files")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be restored")
	cmd.Flags().BoolVarP(&preservePerm, "preserve-perms", "p", true, "Preserve file permissions")

	return cmd
}

func runRestore(repoPath string, opts models.RestoreOptions) error {
	startTime := time.Now()

	// Resolve target path
	targetPath, err := filepath.Abs(opts.TargetPath)
	if err != nil {
		return fmt.Errorf("invalid target path: %w", err)
	}
	opts.TargetPath = targetPath

	// Load config
	cfg := config.DefaultConfig()
	configPath := filepath.Join(repoPath, "config", "snapsync.yaml")
	if loadedCfg, err := config.Load(configPath); err == nil {
		cfg = loadedCfg
	}

	// Setup compression
	var compressor *compress.Compressor
	if cfg.Compression.Enabled {
		compressor, err = compress.New(compress.AlgorithmZstd, cfg.Compression.Level)
		if err != nil {
			return fmt.Errorf("failed to create compressor: %w", err)
		}
		defer compressor.Close()
	}

	// Setup encryption
	var encryptor *crypto.Encryptor
	if cfg.Encryption.Enabled {
		passphrase, err := promptPassword("Enter restore password: ")
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}

		// Load salt
		saltPath := filepath.Join(repoPath, "config", "salt")
		saltData, err := os.ReadFile(saltPath)
		if err != nil {
			return fmt.Errorf("repository not encrypted or salt missing")
		}
		salt, _ := hex.DecodeString(string(saltData))

		encryptor, err = crypto.NewEncryptor(passphrase, salt)
		if err != nil {
			return fmt.Errorf("failed to create encryptor: %w", err)
		}
	}

	// Get snapshot
	mgr, err := snapshot.NewManager(repoPath, compressor, encryptor)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	snap, err := mgr.Get(opts.SnapshotID)
	if err != nil {
		// Try to find by prefix
		snapshots, _ := mgr.List()
		for _, s := range snapshots {
			if len(opts.SnapshotID) >= 8 && s.ID[:8] == opts.SnapshotID[:8] {
				snap = s
				break
			}
		}
		if snap == nil {
			return fmt.Errorf("snapshot not found: %s", opts.SnapshotID)
		}
	}

	// Create CAS
	cas, err := store.NewCAS(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}

	// Create restorer
	restorer := restore.NewRestorer(cas, compressor, encryptor)

	if opts.DryRun {
		fmt.Println("Dry run - no files will be restored")
		fmt.Println()
	}

	// Perform restore
	fmt.Printf("Restoring from snapshot %s...\n", snap.ID[:8])
	fmt.Printf("  Created: %s\n", snap.Timestamp.Format(time.RFC3339))
	fmt.Printf("  Target:  %s\n", opts.TargetPath)
	fmt.Println()

	result, err := restorer.Restore(snap, opts)
	if err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}

	// Print summary
	duration := time.Since(startTime)
	fmt.Println("Restore complete!")
	fmt.Printf("  Files restored: %d\n", result.FilesRestored)
	fmt.Printf("  Bytes restored: %s\n", formatBytes(result.BytesRestored))
	fmt.Printf("  Duration:       %s\n", duration.Round(time.Millisecond))

	if len(result.Errors) > 0 {
		fmt.Printf("\nErrors (%d):\n", len(result.Errors))
		for _, e := range result.Errors {
			fmt.Printf("  %s: %v\n", e.Path, e.Error)
		}
	}

	return nil
}

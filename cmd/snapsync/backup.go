package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/snapsync/snapsync/internal/compress"
	"github.com/snapsync/snapsync/internal/config"
	"github.com/snapsync/snapsync/internal/crypto"
	"github.com/snapsync/snapsync/internal/snapshot"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
)

func backupCmd() *cobra.Command {
	var (
		description string
		encrypt     bool
		noCompress  bool
		exclude     []string
	)

	cmd := &cobra.Command{
		Use:   "backup [source]",
		Short: "Create a backup snapshot",
		Long:  "Creates a new snapshot of the source directory in the repository.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourcePath := args[0]

			if repoPath == "" {
				return fmt.Errorf("repository path required (use --repo)")
			}

			return runBackup(sourcePath, repoPath, description, encrypt, !noCompress, exclude)
		},
	}

	cmd.Flags().StringVarP(&description, "description", "d", "", "Snapshot description")
	cmd.Flags().BoolVarP(&encrypt, "encrypt", "e", false, "Enable encryption")
	cmd.Flags().BoolVar(&noCompress, "no-compress", false, "Disable compression")
	cmd.Flags().StringArrayVarP(&exclude, "exclude", "x", nil, "Exclude patterns")

	return cmd
}

func runBackup(sourcePath, repoPath, description string, encrypt, compressEnabled bool, exclude []string) error {
	startTime := time.Now()

	// Resolve source path
	sourcePath, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("invalid source path: %w", err)
	}

	// Check source exists
	if _, err := os.Stat(sourcePath); err != nil {
		return fmt.Errorf("source not found: %w", err)
	}

	// Load or create config
	cfg := config.DefaultConfig()
	configPath := filepath.Join(repoPath, "config", "snapsync.yaml")
	if loadedCfg, err := config.Load(configPath); err == nil {
		cfg = loadedCfg
	}

	// Merge exclusions
	exclusions := append(cfg.Exclusions, exclude...)

	// Setup compression
	var compressor *compress.Compressor
	if compressEnabled {
		compressor, err = compress.New(compress.AlgorithmZstd, cfg.Compression.Level)
		if err != nil {
			return fmt.Errorf("failed to create compressor: %w", err)
		}
		defer compressor.Close()
	}

	// Setup encryption
	var encryptor *crypto.Encryptor
	if encrypt || cfg.Encryption.Enabled {
		passphrase, err := promptPassword("Enter backup password: ")
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}

		// Check for existing salt
		saltPath := filepath.Join(repoPath, "config", "salt")
		var salt []byte
		if data, err := os.ReadFile(saltPath); err == nil {
			salt, _ = hex.DecodeString(string(data))
		} else {
			salt, _ = crypto.GenerateSalt()
			os.WriteFile(saltPath, []byte(hex.EncodeToString(salt)), 0600)
		}

		encryptor, err = crypto.NewEncryptor(passphrase, salt)
		if err != nil {
			return fmt.Errorf("failed to create encryptor: %w", err)
		}
	}

	// Create snapshot manager
	mgr, err := snapshot.NewManager(repoPath, compressor, encryptor)
	if err != nil {
		return fmt.Errorf("failed to create snapshot manager: %w", err)
	}
	mgr.SetExclusions(exclusions)

	// Get parent snapshot for incremental backup
	var parentID string
	if latest, err := mgr.Latest(); err == nil && latest != nil {
		parentID = latest.ID
	}

	// Create snapshot
	fmt.Printf("Backing up %s...\n", sourcePath)
	snap, err := mgr.Create(sourcePath, description, parentID)
	if err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	// Print summary
	duration := time.Since(startTime)
	fmt.Println()
	fmt.Println("Backup complete!")
	fmt.Printf("  Snapshot ID:    %s\n", snap.ID)
	fmt.Printf("  Files:          %d\n", snap.Tree.FileCount)
	fmt.Printf("  Total size:     %s\n", formatBytes(snap.Stats.TotalSize))
	fmt.Printf("  Stored size:    %s\n", formatBytes(snap.Stats.StoredSize))
	fmt.Printf("  Dedup savings:  %s\n", formatBytes(snap.Stats.DeduplicatedSize))
	fmt.Printf("  New chunks:     %d\n", snap.Stats.NewChunks)
	fmt.Printf("  Duration:       %s\n", duration.Round(time.Millisecond))

	if parentID != "" {
		fmt.Printf("  Added:          %d files\n", snap.Stats.FilesAdded)
		fmt.Printf("  Modified:       %d files\n", snap.Stats.FilesModified)
		fmt.Printf("  Unchanged:      %d files\n", snap.Stats.FilesUnchanged)
	}

	return nil
}

func promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)

	// Try to read password without echo
	if terminal.IsTerminal(int(syscall.Stdin)) {
		password, err := terminal.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return "", err
		}
		return string(password), nil
	}

	// Fallback for non-terminal
	reader := bufio.NewReader(os.Stdin)
	password, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(password), nil
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

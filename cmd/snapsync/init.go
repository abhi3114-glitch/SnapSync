package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/snapsync/snapsync/internal/config"
	"github.com/snapsync/snapsync/pkg/models"
	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	var encrypt bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new backup repository",
		Long:  "Creates a new SnapSync repository at the specified path.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				return fmt.Errorf("repository path required (use --repo)")
			}

			return initRepository(repoPath, encrypt)
		},
	}

	cmd.Flags().BoolVarP(&encrypt, "encrypt", "e", false, "Enable encryption")

	return cmd
}

func initRepository(path string, encrypt bool) error {
	// Create repository directory structure
	dirs := []string{
		filepath.Join(path, "objects"),
		filepath.Join(path, "snapshots"),
		filepath.Join(path, "config"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Create default config
	cfg := config.DefaultConfig()
	cfg.Repository.Path = path
	cfg.Encryption.Enabled = encrypt

	configPath := filepath.Join(path, "config", "snapsync.yaml")
	if err := cfg.Save(configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Create repository info
	info := models.RepositoryInfo{
		Version:   1,
		Encrypted: encrypt,
	}

	infoData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	infoPath := filepath.Join(path, "repo.json")
	if err := os.WriteFile(infoPath, infoData, 0644); err != nil {
		return fmt.Errorf("failed to write repo info: %w", err)
	}

	fmt.Printf("Initialized SnapSync repository at %s\n", path)
	if encrypt {
		fmt.Println("Encryption: enabled (you will be prompted for password on first backup)")
	}

	return nil
}

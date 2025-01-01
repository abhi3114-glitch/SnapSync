package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for SnapSync
type Config struct {
	Repository  RepositoryConfig  `yaml:"repository" json:"repository"`
	Encryption  EncryptionConfig  `yaml:"encryption" json:"encryption"`
	Compression CompressionConfig `yaml:"compression" json:"compression"`
	Cloud       CloudConfig       `yaml:"cloud" json:"cloud"`
	Chunking    ChunkingConfig    `yaml:"chunking" json:"chunking"`
	Exclusions  []string          `yaml:"exclusions" json:"exclusions"`
}

// RepositoryConfig defines repository settings
type RepositoryConfig struct {
	Path     string `yaml:"path" json:"path"`
	AutoInit bool   `yaml:"auto_init" json:"auto_init"`
}

// EncryptionConfig defines encryption settings
type EncryptionConfig struct {
	Enabled   bool   `yaml:"enabled" json:"enabled"`
	Algorithm string `yaml:"algorithm" json:"algorithm"` // aes-256-gcm
	KDF       string `yaml:"kdf" json:"kdf"`             // argon2id
	KeyFile   string `yaml:"key_file" json:"key_file"`   // Optional key file path
}

// CompressionConfig defines compression settings
type CompressionConfig struct {
	Enabled   bool   `yaml:"enabled" json:"enabled"`
	Algorithm string `yaml:"algorithm" json:"algorithm"` // zstd, lz4, none
	Level     int    `yaml:"level" json:"level"`         // Compression level
}

// CloudConfig defines cloud storage settings
type CloudConfig struct {
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	Provider     string `yaml:"provider" json:"provider"` // s3, azure, gcs
	Bucket       string `yaml:"bucket" json:"bucket"`
	Region       string `yaml:"region" json:"region"`
	Endpoint     string `yaml:"endpoint" json:"endpoint"` // For S3-compatible
	AccessKey    string `yaml:"access_key" json:"access_key"`
	SecretKey    string `yaml:"secret_key" json:"secret_key"`
	MaxBandwidth int64  `yaml:"max_bandwidth" json:"max_bandwidth"` // bytes/sec, 0 = unlimited
}

// ChunkingConfig defines content-defined chunking parameters
type ChunkingConfig struct {
	MinSize   int    `yaml:"min_size" json:"min_size"`   // Minimum chunk size
	AvgSize   int    `yaml:"avg_size" json:"avg_size"`   // Target average chunk size
	MaxSize   int    `yaml:"max_size" json:"max_size"`   // Maximum chunk size
	Algorithm string `yaml:"algorithm" json:"algorithm"` // rabin, fixed
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Repository: RepositoryConfig{
			Path:     "",
			AutoInit: true,
		},
		Encryption: EncryptionConfig{
			Enabled:   false,
			Algorithm: "aes-256-gcm",
			KDF:       "argon2id",
		},
		Compression: CompressionConfig{
			Enabled:   true,
			Algorithm: "zstd",
			Level:     3,
		},
		Cloud: CloudConfig{
			Enabled:  false,
			Provider: "s3",
		},
		Chunking: ChunkingConfig{
			MinSize:   512 * 1024,      // 512 KB
			AvgSize:   1024 * 1024,     // 1 MB
			MaxSize:   4 * 1024 * 1024, // 4 MB
			Algorithm: "rabin",
		},
		Exclusions: []string{
			".git",
			".svn",
			"node_modules",
			"__pycache__",
			"*.tmp",
			"*.log",
			".DS_Store",
			"Thumbs.db",
		},
	}
}

// Load reads configuration from a file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()

	ext := filepath.Ext(path)
	switch ext {
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, cfg)
	case ".json":
		err = json.Unmarshal(data, cfg)
	default:
		// Try YAML first, then JSON
		if err = yaml.Unmarshal(data, cfg); err != nil {
			err = json.Unmarshal(data, cfg)
		}
	}

	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// Save writes configuration to a file
func (c *Config) Save(path string) error {
	var data []byte
	var err error

	ext := filepath.Ext(path)
	switch ext {
	case ".json":
		data, err = json.MarshalIndent(c, "", "  ")
	default:
		data, err = yaml.Marshal(c)
	}

	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate chunking sizes
	if c.Chunking.MinSize <= 0 {
		c.Chunking.MinSize = 512 * 1024
	}
	if c.Chunking.AvgSize <= c.Chunking.MinSize {
		c.Chunking.AvgSize = c.Chunking.MinSize * 2
	}
	if c.Chunking.MaxSize <= c.Chunking.AvgSize {
		c.Chunking.MaxSize = c.Chunking.AvgSize * 4
	}

	// Validate compression algorithm
	switch c.Compression.Algorithm {
	case "zstd", "lz4", "none", "":
		// Valid
	default:
		c.Compression.Algorithm = "zstd"
	}

	// Validate compression level
	if c.Compression.Level < 1 {
		c.Compression.Level = 1
	}
	if c.Compression.Level > 19 {
		c.Compression.Level = 19
	}

	return nil
}

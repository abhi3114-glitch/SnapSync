package models

import (
	"os"
	"time"
)

// Chunk represents a data chunk in the content-addressable storage
type Chunk struct {
	Hash   string // SHA-256 hash of the chunk data
	Size   int64  // Size in bytes
	Offset int64  // Offset in the original file
	Data   []byte // Chunk data (only populated during processing)
}

// FileNode represents a file or directory in the snapshot tree
type FileNode struct {
	Path    string      `json:"path"`
	Name    string      `json:"name"`
	IsDir   bool        `json:"is_dir"`
	Mode    os.FileMode `json:"mode"`
	Size    int64       `json:"size"`
	ModTime time.Time   `json:"mod_time"`
	Hash    string      `json:"hash"`   // Full file content hash
	Chunks  []string    `json:"chunks"` // List of chunk hashes
}

// FileTree represents the hierarchical structure of files
type FileTree struct {
	Root      *FileNode            `json:"root"`
	Files     map[string]*FileNode `json:"files"` // Path -> FileNode
	TotalSize int64                `json:"total_size"`
	FileCount int                  `json:"file_count"`
	DirCount  int                  `json:"dir_count"`
}

// Snapshot represents a point-in-time backup
type Snapshot struct {
	ID          string        `json:"id"`
	Timestamp   time.Time     `json:"timestamp"`
	Parent      string        `json:"parent,omitempty"` // Parent snapshot ID for incremental
	Description string        `json:"description,omitempty"`
	Tree        *FileTree     `json:"tree"`
	Stats       SnapshotStats `json:"stats"`
	Encrypted   bool          `json:"encrypted"`
	Compressed  bool          `json:"compressed"`
}

// SnapshotStats contains statistics about a snapshot
type SnapshotStats struct {
	TotalSize        int64         `json:"total_size"`        // Original data size
	StoredSize       int64         `json:"stored_size"`       // Size after dedup/compression
	ChunkCount       int           `json:"chunk_count"`       // Total chunks
	NewChunks        int           `json:"new_chunks"`        // Chunks not in previous snapshots
	DeduplicatedSize int64         `json:"deduplicated_size"` // Bytes saved by dedup
	CompressionRatio float64       `json:"compression_ratio"` // Compression ratio
	Duration         time.Duration `json:"duration"`          // Time to create snapshot
	FilesAdded       int           `json:"files_added"`
	FilesModified    int           `json:"files_modified"`
	FilesDeleted     int           `json:"files_deleted"`
	FilesUnchanged   int           `json:"files_unchanged"`
}

// DiffType represents the type of change between snapshots
type DiffType string

const (
	DiffAdded     DiffType = "added"
	DiffModified  DiffType = "modified"
	DiffDeleted   DiffType = "deleted"
	DiffUnchanged DiffType = "unchanged"
)

// FileDiff represents a difference between two file versions
type FileDiff struct {
	Path      string   `json:"path"`
	Type      DiffType `json:"type"`
	OldHash   string   `json:"old_hash,omitempty"`
	NewHash   string   `json:"new_hash,omitempty"`
	OldSize   int64    `json:"old_size,omitempty"`
	NewSize   int64    `json:"new_size,omitempty"`
	OldChunks []string `json:"old_chunks,omitempty"`
	NewChunks []string `json:"new_chunks,omitempty"`
}

// RestoreOptions configures restore behavior
type RestoreOptions struct {
	SnapshotID     string   // Snapshot to restore from
	TargetPath     string   // Where to restore files
	IncludePattern []string // Glob patterns to include
	ExcludePattern []string // Glob patterns to exclude
	Overwrite      bool     // Overwrite existing files
	PreservePerms  bool     // Preserve file permissions
	DryRun         bool     // Don't actually restore, just show what would happen
}

// BackupOptions configures backup behavior
type BackupOptions struct {
	SourcePath     string   // Directory to backup
	RepoPath       string   // Repository path
	Description    string   // Snapshot description
	ExcludePattern []string // Glob patterns to exclude
	Encrypt        bool     // Enable encryption
	Compress       bool     // Enable compression
	CloudUpload    bool     // Upload to cloud after local backup
}

// RepositoryInfo contains metadata about a backup repository
type RepositoryInfo struct {
	Version       int       `json:"version"`
	Created       time.Time `json:"created"`
	LastBackup    time.Time `json:"last_backup,omitempty"`
	SnapshotCount int       `json:"snapshot_count"`
	TotalSize     int64     `json:"total_size"`
	ChunkCount    int       `json:"chunk_count"`
	Encrypted     bool      `json:"encrypted"`
}

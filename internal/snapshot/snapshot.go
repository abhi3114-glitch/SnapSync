package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/snapsync/snapsync/internal/chunker"
	"github.com/snapsync/snapsync/internal/compress"
	"github.com/snapsync/snapsync/internal/crypto"
	"github.com/snapsync/snapsync/internal/diff"
	"github.com/snapsync/snapsync/internal/scanner"
	"github.com/snapsync/snapsync/internal/store"
	"github.com/snapsync/snapsync/pkg/models"
)

// Manager handles snapshot creation and management
type Manager struct {
	repoPath   string
	cas        *store.CAS
	compressor *compress.Compressor
	encryptor  *crypto.Encryptor
	chunker    *chunker.Chunker
	scanner    *scanner.Scanner
	differ     *diff.Differ
}

// NewManager creates a new snapshot manager
func NewManager(repoPath string, compressor *compress.Compressor, encryptor *crypto.Encryptor) (*Manager, error) {
	cas, err := store.NewCAS(repoPath)
	if err != nil {
		return nil, err
	}

	return &Manager{
		repoPath:   repoPath,
		cas:        cas,
		compressor: compressor,
		encryptor:  encryptor,
		chunker:    chunker.NewDefault(),
		scanner:    scanner.New(nil, 4),
		differ:     diff.New(),
	}, nil
}

// SetExclusions sets file exclusion patterns
func (m *Manager) SetExclusions(patterns []string) {
	m.scanner = scanner.New(patterns, 4)
}

// Create creates a new snapshot of the source path
func (m *Manager) Create(sourcePath, description string, parentID string) (*models.Snapshot, error) {
	startTime := time.Now()

	// Scan source directory
	tree, err := m.scanner.ScanWithHashes(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	// Get parent snapshot for incremental backup
	var parentTree *models.FileTree
	if parentID != "" {
		parent, err := m.Get(parentID)
		if err == nil && parent != nil {
			parentTree = parent.Tree
		}
	}

	// Calculate diff if we have a parent
	var diffResult *diff.DiffResult
	if parentTree != nil {
		diffResult = m.differ.Compare(parentTree, tree)
	}

	// Create snapshot
	snapshot := &models.Snapshot{
		ID:          generateID(),
		Timestamp:   time.Now(),
		Parent:      parentID,
		Description: description,
		Tree:        tree,
		Compressed:  m.compressor != nil,
		Encrypted:   m.encryptor != nil,
	}

	// Process files and store chunks
	var newChunks, totalChunks int
	var storedSize int64

	filesToProcess := tree.Files
	if diffResult != nil {
		// Only process changed files
		filesToProcess = make(map[string]*models.FileNode)
		for _, d := range diffResult.AllChangedFiles() {
			filesToProcess[d.Path] = tree.Files[d.Path]
		}
		// Copy chunks from unchanged files
		for _, d := range diffResult.Unchanged {
			if node, exists := tree.Files[d.Path]; exists {
				node.Chunks = parentTree.Files[d.Path].Chunks
			}
		}
	}

	for relPath, node := range filesToProcess {
		if node.IsDir {
			continue
		}

		// Read and chunk file
		file, err := os.Open(node.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to open %s: %w", relPath, err)
		}

		chunks, err := m.chunker.Chunk(file)
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to chunk %s: %w", relPath, err)
		}

		// Store chunks
		var chunkHashes []string
		for _, chunk := range chunks {
			data := chunk.Data

			// Compress if enabled
			if m.compressor != nil {
				data, err = m.compressor.Compress(data)
				if err != nil {
					return nil, fmt.Errorf("compression failed: %w", err)
				}
			}

			// Encrypt if enabled
			if m.encryptor != nil {
				data, err = m.encryptor.Encrypt(data)
				if err != nil {
					return nil, fmt.Errorf("encryption failed: %w", err)
				}
			}

			// Store in CAS
			if !m.cas.Has(chunk.Hash) {
				_, err = m.cas.Put(data)
				if err != nil {
					return nil, fmt.Errorf("storage failed: %w", err)
				}
				newChunks++
				storedSize += int64(len(data))
			}

			chunkHashes = append(chunkHashes, chunk.Hash)
			totalChunks++
		}

		tree.Files[relPath].Chunks = chunkHashes
	}

	// Update stats
	snapshot.Stats = models.SnapshotStats{
		TotalSize:        tree.TotalSize,
		StoredSize:       storedSize,
		ChunkCount:       totalChunks,
		NewChunks:        newChunks,
		DeduplicatedSize: tree.TotalSize - storedSize,
		Duration:         time.Since(startTime),
	}

	if diffResult != nil {
		snapshot.Stats.FilesAdded = len(diffResult.Added)
		snapshot.Stats.FilesModified = len(diffResult.Modified)
		snapshot.Stats.FilesDeleted = len(diffResult.Deleted)
		snapshot.Stats.FilesUnchanged = len(diffResult.Unchanged)
	} else {
		snapshot.Stats.FilesAdded = tree.FileCount
	}

	// Save snapshot metadata
	if err := m.saveSnapshot(snapshot); err != nil {
		return nil, fmt.Errorf("failed to save snapshot: %w", err)
	}

	return snapshot, nil
}

// Get retrieves a snapshot by ID
func (m *Manager) Get(id string) (*models.Snapshot, error) {
	path := filepath.Join(m.repoPath, "snapshots", id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var snapshot models.Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}

	return &snapshot, nil
}

// List returns all snapshots sorted by timestamp (newest first)
func (m *Manager) List() ([]*models.Snapshot, error) {
	snapshotsDir := filepath.Join(m.repoPath, "snapshots")
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var snapshots []*models.Snapshot
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		id := entry.Name()[:len(entry.Name())-5] // Remove .json
		snap, err := m.Get(id)
		if err != nil {
			continue
		}
		snapshots = append(snapshots, snap)
	}

	// Sort by timestamp (newest first)
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.After(snapshots[j].Timestamp)
	})

	return snapshots, nil
}

// Delete removes a snapshot
func (m *Manager) Delete(id string) error {
	path := filepath.Join(m.repoPath, "snapshots", id+".json")
	return os.Remove(path)
}

// Latest returns the most recent snapshot
func (m *Manager) Latest() (*models.Snapshot, error) {
	snapshots, err := m.List()
	if err != nil {
		return nil, err
	}
	if len(snapshots) == 0 {
		return nil, nil
	}
	return snapshots[0], nil
}

// saveSnapshot writes snapshot metadata to disk
func (m *Manager) saveSnapshot(snapshot *models.Snapshot) error {
	snapshotsDir := filepath.Join(m.repoPath, "snapshots")
	if err := os.MkdirAll(snapshotsDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(snapshotsDir, snapshot.ID+".json")
	return os.WriteFile(path, data, 0644)
}

// generateID creates a unique snapshot ID
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// CAS returns the content-addressable store
func (m *Manager) CAS() *store.CAS {
	return m.cas
}

package diff

import (
	"github.com/snapsync/snapsync/pkg/models"
)

// Differ computes differences between file trees
type Differ struct{}

// New creates a new Differ
func New() *Differ {
	return &Differ{}
}

// DiffResult contains the differences between two trees
type DiffResult struct {
	Added         []*models.FileDiff
	Modified      []*models.FileDiff
	Deleted       []*models.FileDiff
	Unchanged     []*models.FileDiff
	TotalAdded    int64
	TotalDeleted  int64
	TotalModified int64
}

// Compare compares two file trees and returns differences
func (d *Differ) Compare(oldTree, newTree *models.FileTree) *DiffResult {
	result := &DiffResult{}

	// Track processed paths
	processed := make(map[string]bool)

	// Check new tree against old tree
	for path, newNode := range newTree.Files {
		if newNode.IsDir {
			continue
		}

		processed[path] = true

		oldNode, exists := oldTree.Files[path]
		if !exists {
			// File added
			diff := &models.FileDiff{
				Path:      path,
				Type:      models.DiffAdded,
				NewHash:   newNode.Hash,
				NewSize:   newNode.Size,
				NewChunks: newNode.Chunks,
			}
			result.Added = append(result.Added, diff)
			result.TotalAdded += newNode.Size
			continue
		}

		// Check if modified
		if newNode.Hash != oldNode.Hash {
			diff := &models.FileDiff{
				Path:      path,
				Type:      models.DiffModified,
				OldHash:   oldNode.Hash,
				NewHash:   newNode.Hash,
				OldSize:   oldNode.Size,
				NewSize:   newNode.Size,
				OldChunks: oldNode.Chunks,
				NewChunks: newNode.Chunks,
			}
			result.Modified = append(result.Modified, diff)
			result.TotalModified += newNode.Size
		} else {
			// Unchanged
			diff := &models.FileDiff{
				Path:    path,
				Type:    models.DiffUnchanged,
				OldHash: oldNode.Hash,
				NewHash: newNode.Hash,
			}
			result.Unchanged = append(result.Unchanged, diff)
		}
	}

	// Check for deleted files
	for path, oldNode := range oldTree.Files {
		if oldNode.IsDir {
			continue
		}

		if !processed[path] {
			diff := &models.FileDiff{
				Path:      path,
				Type:      models.DiffDeleted,
				OldHash:   oldNode.Hash,
				OldSize:   oldNode.Size,
				OldChunks: oldNode.Chunks,
			}
			result.Deleted = append(result.Deleted, diff)
			result.TotalDeleted += oldNode.Size
		}
	}

	return result
}

// ChunkDiff identifies which chunks need to be stored
type ChunkDiff struct {
	NewChunks      []string // Chunks that don't exist in CAS
	ExistingChunks []string // Chunks that already exist
}

// CompareChunks compares file chunks with existing storage
func (d *Differ) CompareChunks(chunks []string, existsFunc func(hash string) bool) *ChunkDiff {
	diff := &ChunkDiff{}

	for _, hash := range chunks {
		if existsFunc(hash) {
			diff.ExistingChunks = append(diff.ExistingChunks, hash)
		} else {
			diff.NewChunks = append(diff.NewChunks, hash)
		}
	}

	return diff
}

// Stats returns statistics about the diff
func (r *DiffResult) Stats() *models.SnapshotStats {
	return &models.SnapshotStats{
		FilesAdded:     len(r.Added),
		FilesModified:  len(r.Modified),
		FilesDeleted:   len(r.Deleted),
		FilesUnchanged: len(r.Unchanged),
	}
}

// AllChangedFiles returns all files that need processing
func (r *DiffResult) AllChangedFiles() []*models.FileDiff {
	result := make([]*models.FileDiff, 0, len(r.Added)+len(r.Modified))
	result = append(result, r.Added...)
	result = append(result, r.Modified...)
	return result
}

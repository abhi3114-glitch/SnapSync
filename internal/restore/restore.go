package restore

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/snapsync/snapsync/internal/compress"
	"github.com/snapsync/snapsync/internal/crypto"
	"github.com/snapsync/snapsync/internal/store"
	"github.com/snapsync/snapsync/pkg/models"
)

// Restorer handles file restoration from snapshots
type Restorer struct {
	cas        *store.CAS
	compressor *compress.Compressor
	encryptor  *crypto.Encryptor
}

// NewRestorer creates a new Restorer
func NewRestorer(cas *store.CAS, compressor *compress.Compressor, encryptor *crypto.Encryptor) *Restorer {
	return &Restorer{
		cas:        cas,
		compressor: compressor,
		encryptor:  encryptor,
	}
}

// RestoreResult contains the result of a restore operation
type RestoreResult struct {
	FilesRestored int
	BytesRestored int64
	Errors        []RestoreError
}

// RestoreError represents an error during restore
type RestoreError struct {
	Path  string
	Error error
}

// Restore restores files from a snapshot
func (r *Restorer) Restore(snapshot *models.Snapshot, opts models.RestoreOptions) (*RestoreResult, error) {
	result := &RestoreResult{}

	// Create target directory
	if err := os.MkdirAll(opts.TargetPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create target directory: %w", err)
	}

	// Restore each file
	for relPath, node := range snapshot.Tree.Files {
		// Skip directories (they'll be created as needed)
		if node.IsDir {
			continue
		}

		// Check include/exclude patterns
		if !r.shouldRestore(relPath, opts.IncludePattern, opts.ExcludePattern) {
			continue
		}

		targetPath := filepath.Join(opts.TargetPath, relPath)

		// Check if file exists
		if !opts.Overwrite {
			if _, err := os.Stat(targetPath); err == nil {
				continue // Skip existing files
			}
		}

		// Restore the file
		if err := r.restoreFile(node, targetPath, opts); err != nil {
			result.Errors = append(result.Errors, RestoreError{
				Path:  relPath,
				Error: err,
			})
			continue
		}

		result.FilesRestored++
		result.BytesRestored += node.Size
	}

	return result, nil
}

// restoreFile restores a single file
func (r *Restorer) restoreFile(node *models.FileNode, targetPath string, opts models.RestoreOptions) error {
	if opts.DryRun {
		return nil
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Create target file
	file, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Restore chunks
	for _, chunkHash := range node.Chunks {
		data, err := r.cas.Get(chunkHash)
		if err != nil {
			return fmt.Errorf("failed to get chunk %s: %w", chunkHash, err)
		}

		// Decrypt if needed
		if r.encryptor != nil {
			data, err = r.encryptor.Decrypt(data)
			if err != nil {
				return fmt.Errorf("decryption failed: %w", err)
			}
		}

		// Decompress if needed
		if r.compressor != nil {
			data, err = r.compressor.Decompress(data)
			if err != nil {
				return fmt.Errorf("decompression failed: %w", err)
			}
		}

		if _, err := file.Write(data); err != nil {
			return fmt.Errorf("failed to write chunk: %w", err)
		}
	}

	// Restore permissions if requested
	if opts.PreservePerms {
		if err := os.Chmod(targetPath, node.Mode); err != nil {
			// Log but don't fail on permission errors
			fmt.Printf("Warning: failed to set permissions on %s: %v\n", targetPath, err)
		}
	}

	// Restore modification time
	if err := os.Chtimes(targetPath, node.ModTime, node.ModTime); err != nil {
		// Log but don't fail
		fmt.Printf("Warning: failed to set mtime on %s: %v\n", targetPath, err)
	}

	return nil
}

// RestoreToWriter restores a file to an io.Writer
func (r *Restorer) RestoreToWriter(node *models.FileNode, w io.Writer) error {
	for _, chunkHash := range node.Chunks {
		data, err := r.cas.Get(chunkHash)
		if err != nil {
			return fmt.Errorf("failed to get chunk: %w", err)
		}

		// Decrypt if needed
		if r.encryptor != nil {
			data, err = r.encryptor.Decrypt(data)
			if err != nil {
				return fmt.Errorf("decryption failed: %w", err)
			}
		}

		// Decompress if needed
		if r.compressor != nil {
			data, err = r.compressor.Decompress(data)
			if err != nil {
				return fmt.Errorf("decompression failed: %w", err)
			}
		}

		if _, err := w.Write(data); err != nil {
			return err
		}
	}

	return nil
}

// RestoreFile restores a single file by path from a snapshot
func (r *Restorer) RestoreFile(snapshot *models.Snapshot, filePath, targetPath string) error {
	node, exists := snapshot.Tree.Files[filePath]
	if !exists {
		return fmt.Errorf("file not found in snapshot: %s", filePath)
	}

	opts := models.RestoreOptions{
		TargetPath:    targetPath,
		Overwrite:     true,
		PreservePerms: true,
	}

	return r.restoreFile(node, targetPath, opts)
}

// shouldRestore checks if a file should be restored based on patterns
func (r *Restorer) shouldRestore(path string, includes, excludes []string) bool {
	// If no includes specified, include all
	included := len(includes) == 0

	// Check include patterns
	for _, pattern := range includes {
		if r.matchPattern(path, pattern) {
			included = true
			break
		}
	}

	if !included {
		return false
	}

	// Check exclude patterns
	for _, pattern := range excludes {
		if r.matchPattern(path, pattern) {
			return false
		}
	}

	return true
}

// matchPattern checks if path matches a glob pattern
func (r *Restorer) matchPattern(path, pattern string) bool {
	// Try exact match
	if matched, _ := filepath.Match(pattern, path); matched {
		return true
	}

	// Try matching just the filename
	if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
		return true
	}

	// Try matching with ** wildcard (recursive)
	if strings.Contains(pattern, "**") {
		// Simplistic implementation: replace ** with actual path segments
		pattern = strings.ReplaceAll(pattern, "**", "*")
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}

	return false
}

// ListFiles returns a list of files in the snapshot matching the pattern
func (r *Restorer) ListFiles(snapshot *models.Snapshot, pattern string) []*models.FileNode {
	var files []*models.FileNode

	for path, node := range snapshot.Tree.Files {
		if node.IsDir {
			continue
		}
		if pattern == "" || r.matchPattern(path, pattern) {
			files = append(files, node)
		}
	}

	return files
}

// GetFileContent retrieves the content of a file from a snapshot
func (r *Restorer) GetFileContent(snapshot *models.Snapshot, filePath string) ([]byte, error) {
	node, exists := snapshot.Tree.Files[filePath]
	if !exists {
		return nil, fmt.Errorf("file not found: %s", filePath)
	}

	var buf bytes.Buffer
	if err := r.RestoreToWriter(node, &buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/snapsync/snapsync/pkg/models"
)

// Scanner walks a directory tree and builds a FileTree
type Scanner struct {
	exclusions []string
	workers    int
	mu         sync.Mutex
}

// New creates a new Scanner
func New(exclusions []string, workers int) *Scanner {
	if workers <= 0 {
		workers = 4
	}
	return &Scanner{
		exclusions: exclusions,
		workers:    workers,
	}
}

// ScanResult contains the result of a scan operation
type ScanResult struct {
	Tree  *models.FileTree
	Error error
}

// Scan walks the source directory and returns a FileTree
func (s *Scanner) Scan(sourcePath string) (*models.FileTree, error) {
	sourcePath, err := filepath.Abs(sourcePath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, err
	}

	tree := &models.FileTree{
		Files: make(map[string]*models.FileNode),
		Root: &models.FileNode{
			Path:    sourcePath,
			Name:    filepath.Base(sourcePath),
			IsDir:   info.IsDir(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
		},
	}

	// Walk the directory
	err = filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check exclusions
		relPath, _ := filepath.Rel(sourcePath, path)
		if s.shouldExclude(relPath, info.Name()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		node := &models.FileNode{
			Path:    path,
			Name:    info.Name(),
			IsDir:   info.IsDir(),
			Mode:    info.Mode(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}

		if info.IsDir() {
			tree.DirCount++
		} else {
			tree.FileCount++
			tree.TotalSize += info.Size()
		}

		s.mu.Lock()
		tree.Files[relPath] = node
		s.mu.Unlock()

		return nil
	})

	return tree, err
}

// ScanWithHashes scans and computes file hashes
func (s *Scanner) ScanWithHashes(sourcePath string) (*models.FileTree, error) {
	tree, err := s.Scan(sourcePath)
	if err != nil {
		return nil, err
	}

	// Compute hashes for all files
	for relPath, node := range tree.Files {
		if node.IsDir {
			continue
		}

		hash, err := s.hashFile(node.Path)
		if err != nil {
			return nil, err
		}
		tree.Files[relPath].Hash = hash
	}

	return tree, nil
}

// hashFile computes SHA-256 hash of a file
func (s *Scanner) hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// shouldExclude checks if a path should be excluded
func (s *Scanner) shouldExclude(relPath, name string) bool {
	for _, pattern := range s.exclusions {
		// Check exact name match
		if pattern == name {
			return true
		}

		// Check glob pattern
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}

		// Check path pattern
		if strings.Contains(relPath, pattern) {
			return true
		}
	}
	return false
}

// QuickScan performs a fast scan using only mtime/size changes
func (s *Scanner) QuickScan(sourcePath string, previous *models.FileTree) (*models.FileTree, []string, error) {
	tree, err := s.Scan(sourcePath)
	if err != nil {
		return nil, nil, err
	}

	var changedFiles []string

	for relPath, node := range tree.Files {
		if node.IsDir {
			continue
		}

		prevNode, exists := previous.Files[relPath]
		if !exists {
			// New file
			changedFiles = append(changedFiles, relPath)
			continue
		}

		// Check if modified (mtime or size changed)
		if node.ModTime != prevNode.ModTime || node.Size != prevNode.Size {
			changedFiles = append(changedFiles, relPath)
		} else {
			// Copy hash from previous
			tree.Files[relPath].Hash = prevNode.Hash
			tree.Files[relPath].Chunks = prevNode.Chunks
		}
	}

	// Hash only changed files
	for _, relPath := range changedFiles {
		node := tree.Files[relPath]
		hash, err := s.hashFile(node.Path)
		if err != nil {
			return nil, nil, err
		}
		tree.Files[relPath].Hash = hash
	}

	return tree, changedFiles, nil
}

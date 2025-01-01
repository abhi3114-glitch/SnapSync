package backend

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalBackend implements Backend for local filesystem storage
type LocalBackend struct {
	basePath string
}

// NewLocalBackend creates a new local filesystem backend
func NewLocalBackend(basePath string) (*LocalBackend, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backend directory: %w", err)
	}

	return &LocalBackend{
		basePath: basePath,
	}, nil
}

// Put stores data at the specified key
func (l *LocalBackend) Put(key string, data io.Reader, size int64) error {
	path := l.keyToPath(key)

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Create file
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy data
	_, err = io.Copy(file, data)
	if err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

// Get retrieves data by key
func (l *LocalBackend) Get(key string) (io.ReadCloser, error) {
	path := l.keyToPath(key)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("key not found: %s", key)
		}
		return nil, err
	}
	return file, nil
}

// Delete removes data by key
func (l *LocalBackend) Delete(key string) error {
	path := l.keyToPath(key)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// List returns all keys with the given prefix
func (l *LocalBackend) List(prefix string) ([]string, error) {
	var keys []string

	prefixPath := l.keyToPath(prefix)
	baseDir := l.basePath

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}

		key := filepath.ToSlash(relPath)
		if prefix == "" || strings.HasPrefix(path, prefixPath) {
			keys = append(keys, key)
		}

		return nil
	})

	return keys, err
}

// Exists checks if a key exists
func (l *LocalBackend) Exists(key string) (bool, error) {
	path := l.keyToPath(key)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// Size returns the size of an object
func (l *LocalBackend) Size(key string) (int64, error) {
	path := l.keyToPath(key)
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// Close releases resources
func (l *LocalBackend) Close() error {
	return nil
}

// keyToPath converts a key to a filesystem path
func (l *LocalBackend) keyToPath(key string) string {
	// Convert slashes to OS-specific separators
	key = filepath.FromSlash(key)
	return filepath.Join(l.basePath, key)
}

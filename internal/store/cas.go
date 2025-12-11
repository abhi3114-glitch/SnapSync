package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// CAS implements a Content-Addressable Storage system
// Files are stored by their SHA-256 hash, enabling automatic deduplication
type CAS struct {
	basePath string
	mu       sync.RWMutex
	refCount map[string]int // Reference counting for garbage collection
}

// NewCAS creates a new Content-Addressable Storage at the specified path
func NewCAS(basePath string) (*CAS, error) {
	objectsPath := filepath.Join(basePath, "objects")
	if err := os.MkdirAll(objectsPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create CAS directory: %w", err)
	}

	return &CAS{
		basePath: objectsPath,
		refCount: make(map[string]int),
	}, nil
}

// Put stores data and returns its hash
// If the data already exists, it just increments the reference count
func (c *CAS) Put(data []byte) (string, error) {
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already exists
	if c.Has(hashStr) {
		c.refCount[hashStr]++
		return hashStr, nil
	}

	// Write to file
	objPath := c.objectPath(hashStr)
	if err := os.MkdirAll(filepath.Dir(objPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create object directory: %w", err)
	}

	if err := os.WriteFile(objPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write object: %w", err)
	}

	c.refCount[hashStr] = 1
	return hashStr, nil
}

// PutReader stores data from a reader and returns its hash
func (c *CAS) PutReader(reader io.Reader) (string, int64, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", 0, err
	}

	hash, err := c.Put(data)
	return hash, int64(len(data)), err
}

// Get retrieves data by its hash
func (c *CAS) Get(hash string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	objPath := c.objectPath(hash)
	data, err := os.ReadFile(objPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("object not found: %s", hash)
		}
		return nil, err
	}

	// Verify hash
	actualHash := sha256.Sum256(data)
	if hex.EncodeToString(actualHash[:]) != hash {
		return nil, fmt.Errorf("object corruption detected: %s", hash)
	}

	return data, nil
}

// GetReader returns a reader for the object
func (c *CAS) GetReader(hash string) (io.ReadCloser, error) {
	objPath := c.objectPath(hash)
	file, err := os.Open(objPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("object not found: %s", hash)
		}
		return nil, err
	}
	return file, nil
}

// Has checks if an object exists in the store
func (c *CAS) Has(hash string) bool {
	objPath := c.objectPath(hash)
	_, err := os.Stat(objPath)
	return err == nil
}

// Delete removes an object (decrements ref count, deletes when 0)
func (c *CAS) Delete(hash string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if count, exists := c.refCount[hash]; exists {
		if count > 1 {
			c.refCount[hash]--
			return nil
		}
		delete(c.refCount, hash)
	}

	objPath := c.objectPath(hash)
	return os.Remove(objPath)
}

// Size returns the size of an object
func (c *CAS) Size(hash string) (int64, error) {
	objPath := c.objectPath(hash)
	info, err := os.Stat(objPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// List returns all object hashes in the store
func (c *CAS) List() ([]string, error) {
	var hashes []string

	err := filepath.Walk(c.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			// Reconstruct hash from path
			rel, _ := filepath.Rel(c.basePath, path)
			hash := filepath.Base(rel)
			// Validate it's a hex hash
			if len(hash) == 64 {
				hashes = append(hashes, hash)
			}
		}
		return nil
	})

	return hashes, err
}

// Stats returns storage statistics
func (c *CAS) Stats() (objectCount int, totalSize int64, err error) {
	err = filepath.Walk(c.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			objectCount++
			totalSize += info.Size()
		}
		return nil
	})
	return
}

// objectPath returns the filesystem path for an object hash
// Uses first 2 chars as directory for better filesystem performance
func (c *CAS) objectPath(hash string) string {
	if len(hash) < 2 {
		return filepath.Join(c.basePath, hash)
	}
	return filepath.Join(c.basePath, hash[:2], hash)
}

// Verify checks integrity of all objects
func (c *CAS) Verify() ([]string, error) {
	var corrupted []string

	hashes, err := c.List()
	if err != nil {
		return nil, err
	}

	for _, hash := range hashes {
		data, err := os.ReadFile(c.objectPath(hash))
		if err != nil {
			corrupted = append(corrupted, hash)
			continue
		}

		actualHash := sha256.Sum256(data)
		if hex.EncodeToString(actualHash[:]) != hash {
			corrupted = append(corrupted, hash)
		}
	}

	return corrupted, nil
}

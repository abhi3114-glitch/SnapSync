package backend

import (
	"io"
)

// Backend defines the interface for storage backends
type Backend interface {
	// Put stores data with the given key
	Put(key string, data io.Reader, size int64) error

	// Get retrieves data by key
	Get(key string) (io.ReadCloser, error)

	// Delete removes data by key
	Delete(key string) error

	// List returns keys with the given prefix
	List(prefix string) ([]string, error)

	// Exists checks if a key exists
	Exists(key string) (bool, error)

	// Size returns the size of an object
	Size(key string) (int64, error)

	// Close releases backend resources
	Close() error
}

// ProgressCallback is called with upload/download progress
type ProgressCallback func(bytesTransferred int64, totalBytes int64)

// BackendConfig contains common backend configuration
type BackendConfig struct {
	MaxBandwidth int64 // Bytes per second, 0 = unlimited
	OnProgress   ProgressCallback
	Retries      int
}

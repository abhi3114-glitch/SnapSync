package chunker

import (
	"crypto/sha256"
	"encoding/hex"
	"io"

	"github.com/chmduquesne/rollinghash/rabinkarp64"
	"github.com/snapsync/snapsync/pkg/models"
)

const (
	// Default chunk sizes
	DefaultMinSize = 512 * 1024      // 512 KB
	DefaultAvgSize = 1024 * 1024     // 1 MB
	DefaultMaxSize = 4 * 1024 * 1024 // 4 MB

	// Polynomial for Rabin fingerprinting
	polynomial = 0x3DA3358B4DC173
)

// Chunker implements content-defined chunking using Rabin fingerprinting
type Chunker struct {
	minSize int
	avgSize int
	maxSize int
	mask    uint64
}

// New creates a new Chunker with specified size parameters
func New(minSize, avgSize, maxSize int) *Chunker {
	if minSize <= 0 {
		minSize = DefaultMinSize
	}
	if avgSize <= 0 {
		avgSize = DefaultAvgSize
	}
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}

	// Calculate mask for average chunk size
	// We want hash & mask == 0 to occur with probability 1/avgSize
	mask := uint64(avgSize - 1)

	return &Chunker{
		minSize: minSize,
		avgSize: avgSize,
		maxSize: maxSize,
		mask:    mask,
	}
}

// NewDefault creates a Chunker with default parameters
func NewDefault() *Chunker {
	return New(DefaultMinSize, DefaultAvgSize, DefaultMaxSize)
}

// Chunk reads from the reader and produces chunks using content-defined chunking
func (c *Chunker) Chunk(reader io.Reader) ([]*models.Chunk, error) {
	var chunks []*models.Chunk
	var offset int64

	buf := make([]byte, c.maxSize)
	window := make([]byte, 64) // Rolling hash window size
	windowIdx := 0
	windowFull := false

	hasher := rabinkarp64.New()
	hasher.Write(window)

	currentChunk := make([]byte, 0, c.maxSize)

	for {
		n, err := reader.Read(buf)
		if n == 0 {
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			continue
		}

		for i := 0; i < n; i++ {
			b := buf[i]
			currentChunk = append(currentChunk, b)

			// Update rolling hash
			oldByte := window[windowIdx]
			window[windowIdx] = b
			windowIdx = (windowIdx + 1) % len(window)
			if windowIdx == 0 {
				windowFull = true
			}

			if windowFull {
				hasher.Roll(oldByte)
			}

			chunkLen := len(currentChunk)

			// Check for chunk boundary
			shouldSplit := false
			if chunkLen >= c.maxSize {
				// Force split at max size
				shouldSplit = true
			} else if chunkLen >= c.minSize {
				// Check rolling hash for natural boundary
				hash := hasher.Sum64()
				if hash&c.mask == 0 {
					shouldSplit = true
				}
			}

			if shouldSplit {
				chunk := c.createChunk(currentChunk, offset)
				chunks = append(chunks, chunk)
				offset += int64(chunkLen)
				currentChunk = currentChunk[:0]

				// Reset rolling hash
				hasher.Reset()
				hasher.Write(window)
				windowFull = false
				windowIdx = 0
			}
		}

		if err == io.EOF {
			break
		}
	}

	// Handle remaining data
	if len(currentChunk) > 0 {
		chunk := c.createChunk(currentChunk, offset)
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// createChunk creates a new chunk with computed hash
func (c *Chunker) createChunk(data []byte, offset int64) *models.Chunk {
	hash := sha256.Sum256(data)
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	return &models.Chunk{
		Hash:   hex.EncodeToString(hash[:]),
		Size:   int64(len(data)),
		Offset: offset,
		Data:   dataCopy,
	}
}

// ChunkFile reads a file and returns its chunks
func (c *Chunker) ChunkFile(path string) ([]*models.Chunk, error) {
	file, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return c.Chunk(file)
}

// FixedChunker implements fixed-size chunking for comparison/testing
type FixedChunker struct {
	chunkSize int
}

// NewFixed creates a fixed-size chunker
func NewFixed(chunkSize int) *FixedChunker {
	if chunkSize <= 0 {
		chunkSize = DefaultAvgSize
	}
	return &FixedChunker{chunkSize: chunkSize}
}

// Chunk splits data into fixed-size chunks
func (fc *FixedChunker) Chunk(reader io.Reader) ([]*models.Chunk, error) {
	var chunks []*models.Chunk
	var offset int64

	buf := make([]byte, fc.chunkSize)

	for {
		n, err := io.ReadFull(reader, buf)
		if n > 0 {
			hash := sha256.Sum256(buf[:n])
			data := make([]byte, n)
			copy(data, buf[:n])

			chunks = append(chunks, &models.Chunk{
				Hash:   hex.EncodeToString(hash[:]),
				Size:   int64(n),
				Offset: offset,
				Data:   data,
			})
			offset += int64(n)
		}

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	return chunks, nil
}

package compress

import (
	"bytes"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
)

// Algorithm represents a compression algorithm
type Algorithm string

const (
	AlgorithmZstd Algorithm = "zstd"
	AlgorithmLZ4  Algorithm = "lz4"
	AlgorithmNone Algorithm = "none"
)

// Compressor handles data compression and decompression
type Compressor struct {
	algorithm Algorithm
	level     int
	encoder   *zstd.Encoder
	decoder   *zstd.Decoder
}

// New creates a new Compressor with the specified algorithm and level
func New(algorithm Algorithm, level int) (*Compressor, error) {
	c := &Compressor{
		algorithm: algorithm,
		level:     level,
	}

	if algorithm == AlgorithmZstd {
		// Map level 1-19 to zstd levels
		zstdLevel := zstd.EncoderLevelFromZstd(level)
		encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstdLevel))
		if err != nil {
			return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
		}
		c.encoder = encoder

		decoder, err := zstd.NewReader(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
		}
		c.decoder = decoder
	}

	return c, nil
}

// NewDefault creates a Compressor with zstd at level 3
func NewDefault() (*Compressor, error) {
	return New(AlgorithmZstd, 3)
}

// Compress compresses data
func (c *Compressor) Compress(data []byte) ([]byte, error) {
	switch c.algorithm {
	case AlgorithmZstd:
		return c.encoder.EncodeAll(data, nil), nil
	case AlgorithmLZ4:
		return c.compressLZ4(data)
	case AlgorithmNone:
		return data, nil
	default:
		return nil, fmt.Errorf("unknown algorithm: %s", c.algorithm)
	}
}

// Decompress decompresses data
func (c *Compressor) Decompress(data []byte) ([]byte, error) {
	switch c.algorithm {
	case AlgorithmZstd:
		return c.decoder.DecodeAll(data, nil)
	case AlgorithmLZ4:
		return c.decompressLZ4(data)
	case AlgorithmNone:
		return data, nil
	default:
		return nil, fmt.Errorf("unknown algorithm: %s", c.algorithm)
	}
}

// CompressReader returns a compressed reader
func (c *Compressor) CompressReader(r io.Reader) (io.Reader, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	compressed, err := c.Compress(data)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(compressed), nil
}

// DecompressReader returns a decompressed reader
func (c *Compressor) DecompressReader(r io.Reader) (io.Reader, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	decompressed, err := c.Decompress(data)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(decompressed), nil
}

// Ratio calculates the compression ratio
func (c *Compressor) Ratio(original, compressed []byte) float64 {
	if len(original) == 0 {
		return 1.0
	}
	return float64(len(compressed)) / float64(len(original))
}

// Close releases resources
func (c *Compressor) Close() error {
	if c.encoder != nil {
		c.encoder.Close()
	}
	if c.decoder != nil {
		c.decoder.Close()
	}
	return nil
}

// LZ4 compression (simplified implementation using zstd's fast mode)
func (c *Compressor) compressLZ4(data []byte) ([]byte, error) {
	// Use zstd fastest mode as LZ4 alternative
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
	if err != nil {
		return nil, err
	}
	defer encoder.Close()
	return encoder.EncodeAll(data, nil), nil
}

func (c *Compressor) decompressLZ4(data []byte) ([]byte, error) {
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	return decoder.DecodeAll(data, nil)
}

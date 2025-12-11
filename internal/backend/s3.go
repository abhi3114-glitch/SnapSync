package backend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Backend implements Backend for S3-compatible storage
type S3Backend struct {
	client       *s3.Client
	bucket       string
	prefix       string
	maxBandwidth int64
}

// S3Config contains S3 connection configuration
type S3Config struct {
	Bucket       string
	Region       string
	Endpoint     string // For S3-compatible services (MinIO, Backblaze B2)
	AccessKey    string
	SecretKey    string
	Prefix       string // Optional key prefix
	MaxBandwidth int64  // Bytes/sec, 0 = unlimited
}

// NewS3Backend creates a new S3-compatible backend
func NewS3Backend(cfg S3Config) (*S3Backend, error) {
	ctx := context.Background()

	// Build AWS config
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with optional custom endpoint
	var client *s3.Client
	if cfg.Endpoint != "" {
		client = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true // Required for MinIO and similar
		})
	} else {
		client = s3.NewFromConfig(awsCfg)
	}

	return &S3Backend{
		client:       client,
		bucket:       cfg.Bucket,
		prefix:       cfg.Prefix,
		maxBandwidth: cfg.MaxBandwidth,
	}, nil
}

// Put uploads data to S3
func (s *S3Backend) Put(key string, data io.Reader, size int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	fullKey := s.prefixKey(key)

	// Read all data (needed for ContentLength)
	buf, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}

	// Apply bandwidth limiting if configured
	reader := io.Reader(bytes.NewReader(buf))
	if s.maxBandwidth > 0 {
		reader = newThrottledReader(bytes.NewReader(buf), s.maxBandwidth)
	}

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(fullKey),
		Body:          reader,
		ContentLength: aws.Int64(int64(len(buf))),
	})

	if err != nil {
		return fmt.Errorf("S3 upload failed: %w", err)
	}

	return nil
}

// Get downloads data from S3
func (s *S3Backend) Get(key string) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	fullKey := s.prefixKey(key)

	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		return nil, fmt.Errorf("S3 download failed: %w", err)
	}

	return resp.Body, nil
}

// Delete removes an object from S3
func (s *S3Backend) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fullKey := s.prefixKey(key)

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})

	if err != nil {
		return fmt.Errorf("S3 delete failed: %w", err)
	}

	return nil
}

// List returns all keys with the given prefix
func (s *S3Backend) List(prefix string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fullPrefix := s.prefixKey(prefix)
	var keys []string

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(fullPrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("S3 list failed: %w", err)
		}

		for _, obj := range page.Contents {
			key := strings.TrimPrefix(*obj.Key, s.prefix)
			keys = append(keys, strings.TrimPrefix(key, "/"))
		}
	}

	return keys, nil
}

// Exists checks if an object exists in S3
func (s *S3Backend) Exists(key string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fullKey := s.prefixKey(key)

	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})

	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// Size returns the size of an object
func (s *S3Backend) Size(key string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fullKey := s.prefixKey(key)

	resp, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})

	if err != nil {
		return 0, err
	}

	return *resp.ContentLength, nil
}

// Close releases resources
func (s *S3Backend) Close() error {
	return nil
}

// prefixKey adds the configured prefix to a key
func (s *S3Backend) prefixKey(key string) string {
	if s.prefix == "" {
		return key
	}
	return s.prefix + "/" + key
}

// throttledReader implements bandwidth limiting
type throttledReader struct {
	reader      io.Reader
	bytesPerSec int64
	lastRead    time.Time
	bytesRead   int64
}

func newThrottledReader(r io.Reader, bytesPerSec int64) *throttledReader {
	return &throttledReader{
		reader:      r,
		bytesPerSec: bytesPerSec,
		lastRead:    time.Now(),
	}
}

func (t *throttledReader) Read(p []byte) (int, error) {
	// Calculate required delay to maintain bandwidth limit
	elapsed := time.Since(t.lastRead)
	expectedTime := time.Duration(float64(t.bytesRead) / float64(t.bytesPerSec) * float64(time.Second))

	if expectedTime > elapsed {
		time.Sleep(expectedTime - elapsed)
	}

	n, err := t.reader.Read(p)
	t.bytesRead += int64(n)
	t.lastRead = time.Now()

	return n, err
}

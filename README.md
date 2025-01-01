# SnapSync

A high-performance, snapshot-based backup system written in Go. SnapSync provides efficient file backups with content-defined chunking, deduplication, compression, encryption, and cloud storage support.

## Features

### Content-Defined Chunking
Uses Rabin fingerprinting to intelligently split files into variable-size chunks. This enables efficient deduplication even when files are modified, as only changed portions need to be stored.

### Deduplication
Content-addressable storage ensures identical data blocks are stored only once, significantly reducing storage requirements when backing up similar files or multiple versions of the same data.

### Compression
Built-in ZSTD compression with configurable levels (1-19) reduces storage footprint. LZ4 is also available for scenarios prioritizing speed over compression ratio.

### Encryption
AES-256-GCM authenticated encryption protects data at rest. Keys are derived from user passphrases using Argon2id, a memory-hard function resistant to GPU-based attacks.

### Cloud Storage
S3-compatible backend supports AWS S3, MinIO, Backblaze B2, and other compatible services. Includes bandwidth throttling for controlled upload speeds.

### Incremental Backups
Delta encoding between snapshots means only changed chunks are processed and stored, making subsequent backups significantly faster.

## Installation

```bash
git clone https://github.com/abhi3114-glitch/SnapSync.git
cd SnapSync
go build -o snapsync ./cmd/snapsync
```

## Quick Start

### Initialize a Repository

```bash
# Create a new backup repository
snapsync init --repo /path/to/repo

# With encryption enabled
snapsync init --repo /path/to/repo --encrypt
```

### Create a Backup

```bash
# Basic backup
snapsync backup /path/to/data --repo /path/to/repo

# With description and encryption
snapsync backup /path/to/data --repo /path/to/repo --encrypt -d "Daily backup"

# Exclude specific patterns
snapsync backup /path/to/data --repo /path/to/repo -x "*.log" -x "node_modules"
```

### List Snapshots

```bash
# List all snapshots
snapsync list --repo /path/to/repo

# View files in a specific snapshot
snapsync list <snapshot-id> --files --repo /path/to/repo
```

### Restore Files

```bash
# Restore entire snapshot
snapsync restore <snapshot-id> /path/to/target --repo /path/to/repo

# Restore specific files by pattern
snapsync restore <snapshot-id> /path/to/target --include "*.docx" --repo /path/to/repo

# Preview what would be restored
snapsync restore <snapshot-id> /path/to/target --dry-run --repo /path/to/repo
```

### Check Repository Status

```bash
snapsync status --repo /path/to/repo
```

## Architecture

```
SnapSync/
├── cmd/snapsync/          # CLI application
│   ├── main.go            # Entry point
│   ├── init.go            # Repository initialization
│   ├── backup.go          # Backup command
│   ├── restore.go         # Restore command
│   ├── list.go            # List snapshots
│   └── status.go          # Repository status
├── internal/
│   ├── chunker/           # Rabin fingerprint chunking
│   ├── store/             # Content-addressable storage
│   ├── snapshot/          # Snapshot management
│   ├── scanner/           # File system scanner
│   ├── diff/              # Delta encoding
│   ├── compress/          # ZSTD/LZ4 compression
│   ├── crypto/            # AES-GCM encryption
│   ├── backend/           # Storage backends
│   ├── restore/           # File restoration
│   └── config/            # Configuration management
└── pkg/models/            # Data structures
```

## How It Works

### Backup Process

1. The scanner walks the source directory and collects file metadata
2. For incremental backups, the diff engine compares with the parent snapshot to identify changes
3. Changed files are split into chunks using content-defined boundaries
4. Each chunk is hashed with SHA-256 for content addressing
5. New chunks are compressed with ZSTD
6. If encryption is enabled, chunks are encrypted with AES-256-GCM
7. Chunks are stored in the content-addressable store
8. A snapshot record captures the file tree and chunk references

### Deduplication

Files are split at content-defined boundaries using a rolling hash algorithm. Each chunk is identified by its SHA-256 hash. When identical content appears across files or versions, only one copy is stored.

### Security

- Key derivation uses Argon2id with recommended parameters (64MB memory, 3 iterations)
- Each chunk is encrypted with a unique nonce to prevent pattern analysis
- Password verification without exposing the derived key

## Configuration

Configuration is stored in the repository at `config/snapsync.yaml`:

```yaml
repository:
  path: /path/to/repo

encryption:
  enabled: true
  algorithm: aes-256-gcm
  kdf: argon2id

compression:
  enabled: true
  algorithm: zstd
  level: 3

chunking:
  min_size: 524288    # 512 KB
  avg_size: 1048576   # 1 MB
  max_size: 4194304   # 4 MB

exclusions:
  - .git
  - node_modules
  - __pycache__
  - "*.tmp"
  - "*.log"
```

## Cloud Storage Configuration

For S3-compatible backends:

```yaml
cloud:
  enabled: true
  provider: s3
  bucket: my-backup-bucket
  region: us-east-1
  endpoint: ""  # Custom endpoint for MinIO/B2
  access_key: YOUR_ACCESS_KEY
  secret_key: YOUR_SECRET_KEY
  max_bandwidth: 0  # bytes/sec, 0 = unlimited
```

## Command Reference

| Command | Description |
|---------|-------------|
| `snapsync init` | Initialize a new repository |
| `snapsync backup` | Create a backup snapshot |
| `snapsync restore` | Restore files from a snapshot |
| `snapsync list` | List snapshots or browse files |
| `snapsync status` | Show repository statistics |

### Global Flags

| Flag | Description |
|------|-------------|
| `--repo, -r` | Repository path |
| `--config, -c` | Configuration file path |
| `--verbose, -v` | Verbose output |

## Dependencies

- github.com/spf13/cobra - CLI framework
- github.com/chmduquesne/rollinghash - Rabin fingerprinting
- github.com/klauspost/compress - ZSTD compression
- github.com/aws/aws-sdk-go-v2 - AWS S3 client
- golang.org/x/crypto - Argon2id, terminal handling

## License

MIT License

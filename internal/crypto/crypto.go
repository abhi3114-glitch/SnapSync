package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	// Argon2id parameters (OWASP recommended)
	argon2Time    = 3
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4
	argon2KeyLen  = 32 // 256 bits for AES-256

	// Salt size
	saltSize = 32

	// Nonce size for AES-GCM
	nonceSize = 12
)

// Encryptor handles encryption and decryption using AES-256-GCM
type Encryptor struct {
	key    []byte
	salt   []byte
	cipher cipher.AEAD
}

// NewEncryptor creates a new Encryptor from a passphrase
func NewEncryptor(passphrase string, salt []byte) (*Encryptor, error) {
	if len(salt) == 0 {
		salt = make([]byte, saltSize)
		if _, err := rand.Read(salt); err != nil {
			return nil, fmt.Errorf("failed to generate salt: %w", err)
		}
	}

	// Derive key using Argon2id
	key := argon2.IDKey(
		[]byte(passphrase),
		salt,
		argon2Time,
		argon2Memory,
		argon2Threads,
		argon2KeyLen,
	)

	// Create AES-GCM cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &Encryptor{
		key:    key,
		salt:   salt,
		cipher: gcm,
	}, nil
}

// Salt returns the salt used for key derivation
func (e *Encryptor) Salt() []byte {
	return e.salt
}

// Encrypt encrypts plaintext and returns ciphertext with prepended nonce
func (e *Encryptor) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Seal prepends the ciphertext to the nonce
	ciphertext := e.cipher.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext (with prepended nonce)
func (e *Encryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce := ciphertext[:nonceSize]
	ciphertext = ciphertext[nonceSize:]

	plaintext, err := e.cipher.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// EncryptReader returns an encrypted reader
func (e *Encryptor) EncryptReader(r io.Reader) (io.Reader, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	encrypted, err := e.Encrypt(data)
	if err != nil {
		return nil, err
	}

	return &encryptedReader{data: encrypted}, nil
}

// DecryptReader returns a decrypted reader
func (e *Encryptor) DecryptReader(r io.Reader) (io.Reader, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	decrypted, err := e.Decrypt(data)
	if err != nil {
		return nil, err
	}

	return &encryptedReader{data: decrypted}, nil
}

type encryptedReader struct {
	data []byte
	pos  int
}

func (r *encryptedReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// GenerateSalt generates a new random salt
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// HashPassword creates a verifiable hash of the password
// Used to verify correct password without storing key
func HashPassword(passphrase string, salt []byte) string {
	key := argon2.IDKey(
		[]byte(passphrase),
		salt,
		argon2Time,
		argon2Memory,
		argon2Threads,
		argon2KeyLen,
	)
	hash := sha256.Sum256(key)
	return hex.EncodeToString(hash[:])
}

// EncryptionHeader contains metadata needed for decryption
type EncryptionHeader struct {
	Version      int    `json:"version"`
	Algorithm    string `json:"algorithm"`
	KDF          string `json:"kdf"`
	Salt         string `json:"salt"`          // Hex-encoded
	PasswordHash string `json:"password_hash"` // For verification
}

// NewEncryptionHeader creates header metadata
func NewEncryptionHeader(salt []byte, passphrase string) *EncryptionHeader {
	return &EncryptionHeader{
		Version:      1,
		Algorithm:    "aes-256-gcm",
		KDF:          "argon2id",
		Salt:         hex.EncodeToString(salt),
		PasswordHash: HashPassword(passphrase, salt),
	}
}

// VerifyPassword checks if the password is correct
func (h *EncryptionHeader) VerifyPassword(passphrase string) bool {
	salt, _ := hex.DecodeString(h.Salt)
	return HashPassword(passphrase, salt) == h.PasswordHash
}

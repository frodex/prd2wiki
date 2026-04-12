package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var hexHash64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Store writes content-addressed blobs under dataDir/blobs/{aa}/{fullhash}.
type Store struct {
	dataDir string
}

// NewStore creates a blob store rooted at dataDir/blobs.
func NewStore(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

// ValidHash reports whether s is a 64-char lowercase hex SHA-256 digest.
func ValidHash(s string) bool {
	return hexHash64.MatchString(s)
}

// Put reads all of r, stores under blobs/{aa}/{hash}, returns lowercase hex hash.
func (s *Store) Put(r io.Reader) (hash string, err error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return s.PutBytes(data)
}

// PutBytes stores pre-read content and returns the lowercase hex SHA-256.
func (s *Store) PutBytes(data []byte) (hash string, err error) {
	sum := sha256.Sum256(data)
	hash = hex.EncodeToString(sum[:])
	sub := filepath.Join(s.dataDir, "blobs", hash[:2], hash)
	if err := os.MkdirAll(filepath.Dir(sub), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(sub); err == nil {
		return hash, nil
	}
	if err := os.WriteFile(sub, data, 0o644); err != nil {
		return "", err
	}
	return hash, nil
}

// Open returns a reader for an existing blob, or an error.
func (s *Store) Open(hash string) (*os.File, error) {
	if !ValidHash(hash) {
		return nil, fmt.Errorf("invalid hash")
	}
	p := filepath.Join(s.dataDir, "blobs", hash[:2], hash)
	return os.Open(p)
}

// FilePath returns the filesystem path for a valid hash, or empty string if invalid.
func (s *Store) FilePath(hash string) string {
	if !ValidHash(hash) {
		return ""
	}
	return filepath.Join(s.dataDir, "blobs", hash[:2], hash)
}

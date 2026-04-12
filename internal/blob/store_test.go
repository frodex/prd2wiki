package blob

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)
	data := []byte("hello blob world")
	h, err := s.PutBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidHash(h) {
		t.Fatalf("invalid hash %q", h)
	}
	buf, err := os.ReadFile(s.FilePath(h))
	if err != nil {
		t.Fatal(err)
	}
	if string(buf) != string(data) {
		t.Fatalf("content mismatch")
	}
	p := filepath.Join(tmp, "blobs", h[:2], h)
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
}

func TestValidHash(t *testing.T) {
	if !ValidHash(strings.Repeat("a", 64)) {
		t.Error("expected valid")
	}
	if ValidHash(strings.Repeat("g", 64)) {
		t.Error("expected invalid")
	}
	if ValidHash(strings.Repeat("a", 63)) {
		t.Error("expected invalid length")
	}
}

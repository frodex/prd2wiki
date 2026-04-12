package tree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateLinkFileLibrarianHead(t *testing.T) {
	dir := t.TempDir()
	const rel = "proj/a-page"
	p := filepath.Join(dir, filepath.FromSlash(rel)+".link")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	pageUUID := "550e8400-e29b-41d4-a716-446655440000"
	if err := os.WriteFile(p, []byte(pageUUID+"\n\nMy Title\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateLinkFileLibrarianHead(dir, rel, pageUUID, "mem_abc123"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("lines: %q", string(b))
	}
	if lines[0] != pageUUID || lines[1] != "mem_abc123" || lines[2] != "My Title" {
		t.Fatalf("got %v", lines)
	}
}

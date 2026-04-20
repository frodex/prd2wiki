package tree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMovePage(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "acme")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".uuid"), []byte("proj-uuid\nAcme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(proj, "old-slug.link")
	content := "page-uuid\n\nTitle\n"
	if err := os.WriteFile(oldPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MovePage(root, "acme/old-slug", "acme/new-slug"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("expected old link removed")
	}
	b, err := os.ReadFile(filepath.Join(proj, "new-slug.link"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != content {
		t.Fatalf("content: got %q want %q", b, content)
	}
}

func TestRenamePage(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "wiki")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".uuid"), []byte("u\nW\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "a.link"), []byte("id\n\nT\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RenamePage(root, "wiki", "a", "b"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(proj, "b.link")); err != nil {
		t.Fatal(err)
	}
}

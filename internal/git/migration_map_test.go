package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMigrationAliases(t *testing.T) {
	dir := t.TempDir()
	raw := `{
  "pages": {
    "a": {"old_path": "pages/86/abc.md", "new_path": "pages/uuid-1.md"}
  }
}`
	if err := os.WriteFile(filepath.Join(dir, MigrationMapFile), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadMigrationAliases(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := m["pages/uuid-1.md"]; len(got) != 1 || got[0] != "pages/86/abc.md" {
		t.Fatalf("got %#v", m)
	}
}

func TestLoadMigrationAliasesMissing(t *testing.T) {
	m, err := LoadMigrationAliases(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 0 {
		t.Fatalf("want empty, got %d", len(m))
	}
}

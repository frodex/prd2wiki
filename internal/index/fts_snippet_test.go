package index

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFTSSnippetsBody(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.db")
	db, err := OpenDatabase(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`INSERT INTO pages (id, title, type, status, path, project, trust_level, tags)
VALUES ('p1', 'T', 'task', 'draft', 'pages/p1.md', 'proj', 0, '')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO pages_fts (id, title, body, tags) VALUES ('p1', 'T', 'alpha beta needle gamma delta', '')`)
	if err != nil {
		t.Fatal(err)
	}

	s := NewSearcher(db)
	out, err := s.FTSSnippetsBody("proj", []string{"p1"}, "needle")
	if err != nil {
		t.Fatal(err)
	}
	snip, ok := out["p1"]
	if !ok || snip == "" {
		t.Fatalf("snippet missing: %#v", out)
	}
	if !strings.Contains(strings.ToLower(snip), "needle") {
		t.Fatalf("snippet %q should mention needle", snip)
	}
}

package index

import (
	"database/sql"
	"path/filepath"
	"testing"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/schema"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDatabase(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDatabase: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestIndexPage(t *testing.T) {
	db := openTestDB(t)
	ix := NewIndexer(db)

	fm := &schema.Frontmatter{
		ID:     "doc-001",
		Title:  "Test Page",
		Type:   "decision",
		Status: "approved",
		Tags:   []string{"alpha", "beta"},
		Provenance: schema.Provenance{
			Sources: []schema.Source{
				{Ref: "doc-ref-001", Version: 1, Checksum: "abc123", Status: "valid"},
				{Ref: "doc-ref-002", Version: 2, Status: "superseded"},
			},
		},
	}
	body := []byte("# Test Page\n\nSome content here.")

	err := ix.IndexPage("myproject", "main", "pages/doc-001.md", fm, body)
	if err != nil {
		t.Fatalf("IndexPage: %v", err)
	}

	// Verify page in pages table
	var id, title, typ, status, project, branch, tags string
	err = db.QueryRow(
		"SELECT id, title, type, status, project, branch, tags FROM pages WHERE id = ?",
		"doc-001",
	).Scan(&id, &title, &typ, &status, &project, &branch, &tags)
	if err != nil {
		t.Fatalf("query pages: %v", err)
	}
	if id != "doc-001" {
		t.Errorf("id: got %q, want %q", id, "doc-001")
	}
	if title != "Test Page" {
		t.Errorf("title: got %q, want %q", title, "Test Page")
	}
	if typ != "decision" {
		t.Errorf("type: got %q, want %q", typ, "decision")
	}
	if status != "approved" {
		t.Errorf("status: got %q, want %q", status, "approved")
	}
	if project != "myproject" {
		t.Errorf("project: got %q, want %q", project, "myproject")
	}
	if branch != "main" {
		t.Errorf("branch: got %q, want %q", branch, "main")
	}
	if tags != "alpha,beta" {
		t.Errorf("tags: got %q, want %q", tags, "alpha,beta")
	}

	// Verify provenance edges
	var count int
	err = db.QueryRow(
		"SELECT COUNT(*) FROM provenance_edges WHERE source_page = ?",
		"doc-001",
	).Scan(&count)
	if err != nil {
		t.Fatalf("query provenance_edges: %v", err)
	}
	if count != 2 {
		t.Errorf("provenance_edges count: got %d, want 2", count)
	}

	// Check edge details
	var targetRef string
	var version int
	var checksum, edgeStatus string
	err = db.QueryRow(
		"SELECT target_ref, target_version, target_checksum, status FROM provenance_edges WHERE source_page = ? AND target_ref = ?",
		"doc-001", "doc-ref-001",
	).Scan(&targetRef, &version, &checksum, &edgeStatus)
	if err != nil {
		t.Fatalf("query provenance edge: %v", err)
	}
	if targetRef != "doc-ref-001" {
		t.Errorf("target_ref: got %q, want %q", targetRef, "doc-ref-001")
	}
	if version != 1 {
		t.Errorf("target_version: got %d, want 1", version)
	}
	if checksum != "abc123" {
		t.Errorf("target_checksum: got %q, want %q", checksum, "abc123")
	}
	if edgeStatus != "valid" {
		t.Errorf("status: got %q, want %q", edgeStatus, "valid")
	}
}

func TestIndexPageUpsert(t *testing.T) {
	db := openTestDB(t)
	ix := NewIndexer(db)

	fm := &schema.Frontmatter{
		ID:     "doc-002",
		Title:  "Original Title",
		Type:   "policy",
		Status: "draft",
	}

	if err := ix.IndexPage("proj", "main", "doc-002.md", fm, []byte("original")); err != nil {
		t.Fatalf("first IndexPage: %v", err)
	}

	// Update
	fm.Title = "Updated Title"
	fm.Status = "approved"
	fm.Provenance = schema.Provenance{
		Sources: []schema.Source{{Ref: "src-1", Status: "valid"}},
	}
	if err := ix.IndexPage("proj", "main", "doc-002.md", fm, []byte("updated")); err != nil {
		t.Fatalf("second IndexPage: %v", err)
	}

	var title, status string
	err := db.QueryRow("SELECT title, status FROM pages WHERE id = ?", "doc-002").Scan(&title, &status)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if title != "Updated Title" {
		t.Errorf("title after upsert: got %q, want %q", title, "Updated Title")
	}
	if status != "approved" {
		t.Errorf("status after upsert: got %q, want %q", status, "approved")
	}

	// After upsert, provenance should be refreshed (1 edge, not 2)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM provenance_edges WHERE source_page = ?", "doc-002").Scan(&count); err != nil {
		t.Fatalf("count provenance: %v", err)
	}
	if count != 1 {
		t.Errorf("provenance count after upsert: got %d, want 1", count)
	}
}

func TestRemovePage(t *testing.T) {
	db := openTestDB(t)
	ix := NewIndexer(db)

	fm := &schema.Frontmatter{
		ID:     "doc-rm",
		Title:  "To Remove",
		Type:   "policy",
		Status: "draft",
		Provenance: schema.Provenance{
			Sources: []schema.Source{{Ref: "ref-rm", Status: "valid"}},
		},
	}

	if err := ix.IndexPage("proj", "main", "doc-rm.md", fm, []byte("body")); err != nil {
		t.Fatalf("IndexPage: %v", err)
	}

	// Verify it exists
	var id string
	if err := db.QueryRow("SELECT id FROM pages WHERE id = ?", "doc-rm").Scan(&id); err != nil {
		t.Fatalf("page should exist before remove: %v", err)
	}

	// Remove
	if err := ix.RemovePage("doc-rm"); err != nil {
		t.Fatalf("RemovePage: %v", err)
	}

	// Verify page is gone
	err := db.QueryRow("SELECT id FROM pages WHERE id = ?", "doc-rm").Scan(&id)
	if err != sql.ErrNoRows {
		t.Errorf("page should be gone after remove, got err=%v id=%q", err, id)
	}

	// Verify provenance edges are gone
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM provenance_edges WHERE source_page = ?", "doc-rm").Scan(&count); err != nil {
		t.Fatalf("count provenance: %v", err)
	}
	if count != 0 {
		t.Errorf("provenance edges should be gone after remove, got %d", count)
	}
}

func TestRebuildFromRepo(t *testing.T) {
	db := openTestDB(t)
	ix := NewIndexer(db)

	// Set up a git repo with some pages
	dir := t.TempDir()
	repo, err := wgit.InitRepo(dir, "testproj")
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	pages := []struct {
		path string
		fm   *schema.Frontmatter
		body []byte
	}{
		{
			path: "pages/alpha.md",
			fm: &schema.Frontmatter{
				ID: "alpha-001", Title: "Alpha", Type: "decision", Status: "approved",
				Provenance: schema.Provenance{
					Sources: []schema.Source{{Ref: "src-alpha", Status: "valid"}},
				},
			},
			body: []byte("Alpha page body."),
		},
		{
			path: "pages/beta.md",
			fm: &schema.Frontmatter{
				ID: "beta-001", Title: "Beta", Type: "policy", Status: "draft",
			},
			body: []byte("Beta page body."),
		},
		{
			path: "notes/gamma.md",
			fm: &schema.Frontmatter{
				ID: "gamma-001", Title: "Gamma", Type: "note", Status: "draft",
				Tags: []string{"important"},
			},
			body: []byte("Gamma note body."),
		},
	}

	for _, p := range pages {
		if err := repo.WritePageWithMeta("main", p.path, p.fm, p.body, "add "+p.path, "test"); err != nil {
			t.Fatalf("WritePageWithMeta %s: %v", p.path, err)
		}
	}

	// Also write a non-.md file to ensure it's skipped
	if err := repo.WritePage("main", "README.txt", []byte("readme"), "add readme", "test"); err != nil {
		t.Fatalf("WritePage README.txt: %v", err)
	}

	// Run rebuild
	if err := ix.RebuildFromRepo("testproj", repo, "main"); err != nil {
		t.Fatalf("RebuildFromRepo: %v", err)
	}

	// Verify 3 pages are indexed (not the .txt file)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM pages WHERE project = ?", "testproj").Scan(&count); err != nil {
		t.Fatalf("count pages: %v", err)
	}
	if count != 3 {
		t.Errorf("page count: got %d, want 3", count)
	}

	// Verify specific page
	var title, branch string
	if err := db.QueryRow("SELECT title, branch FROM pages WHERE id = ?", "alpha-001").Scan(&title, &branch); err != nil {
		t.Fatalf("query alpha: %v", err)
	}
	if title != "Alpha" {
		t.Errorf("alpha title: got %q, want %q", title, "Alpha")
	}
	if branch != "main" {
		t.Errorf("alpha branch: got %q, want %q", branch, "main")
	}

	// Verify provenance edge for alpha (has 1 source)
	var edgeCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM provenance_edges WHERE source_page = ?", "alpha-001").Scan(&edgeCount); err != nil {
		t.Fatalf("count alpha edges: %v", err)
	}
	if edgeCount != 1 {
		t.Errorf("alpha edge count: got %d, want 1", edgeCount)
	}
}

func TestRebuildFromRepoClears(t *testing.T) {
	db := openTestDB(t)
	ix := NewIndexer(db)

	dir := t.TempDir()
	repo, err := wgit.InitRepo(dir, "clearproj")
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	fm := &schema.Frontmatter{ID: "stale-001", Title: "Stale", Type: "policy", Status: "draft"}
	if err := repo.WritePageWithMeta("main", "stale.md", fm, []byte("body"), "add", "test"); err != nil {
		t.Fatalf("WritePageWithMeta: %v", err)
	}

	// First rebuild
	if err := ix.RebuildFromRepo("clearproj", repo, "main"); err != nil {
		t.Fatalf("first RebuildFromRepo: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM pages WHERE project = ?", "clearproj").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 page after first rebuild, got %d", count)
	}

	// Write a different page (simulating a page being replaced)
	fm2 := &schema.Frontmatter{ID: "new-001", Title: "New", Type: "decision", Status: "approved"}
	if err := repo.WritePageWithMeta("main", "new.md", fm2, []byte("body"), "add new", "test"); err != nil {
		t.Fatalf("WritePageWithMeta new: %v", err)
	}

	// Second rebuild should clear old and re-index both
	if err := ix.RebuildFromRepo("clearproj", repo, "main"); err != nil {
		t.Fatalf("second RebuildFromRepo: %v", err)
	}

	if err := db.QueryRow("SELECT COUNT(*) FROM pages WHERE project = ?", "clearproj").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 pages after second rebuild, got %d", count)
	}
}

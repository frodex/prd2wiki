package index

import (
	"path/filepath"
	"testing"

	"github.com/frodex/prd2wiki/internal/schema"
)

// setupSearchDB creates a temp SQLite DB, seeds 4 pages, and returns a Searcher and Indexer.
func setupSearchDB(t *testing.T) (*Searcher, *Indexer) {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDatabase(filepath.Join(dir, "search_test.db"))
	if err != nil {
		t.Fatalf("OpenDatabase: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ix := NewIndexer(db)

	pages := []struct {
		id     string
		title  string
		typ    string
		status string
		tags   []string
	}{
		{"P-001", "Auth Requirement", "requirement", "active", []string{"auth", "security"}},
		{"P-002", "Session Concept", "concept", "active", []string{"session"}},
		{"P-003", "API Reference", "reference", "draft", []string{"api"}},
		{"P-004", "Deprecated Auth Req", "requirement", "deprecated", []string{"auth"}},
	}

	for _, p := range pages {
		fm := &schema.Frontmatter{
			ID:     p.id,
			Title:  p.title,
			Type:   p.typ,
			Status: p.status,
			Tags:   p.tags,
		}
		if err := ix.IndexPage("testproj", "main", "pages/"+p.id+".md", fm, []byte("body")); err != nil {
			t.Fatalf("IndexPage %s: %v", p.id, err)
		}
	}

	return NewSearcher(db), ix
}

func TestListAll(t *testing.T) {
	s, _ := setupSearchDB(t)
	results, err := s.ListAll("testproj")
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(results) != 4 {
		t.Errorf("ListAll: got %d results, want 4", len(results))
	}
}

func TestSearchByType(t *testing.T) {
	s, _ := setupSearchDB(t)
	results, err := s.ByType("testproj", "requirement")
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("ByType(requirement): got %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.Type != "requirement" {
			t.Errorf("ByType: expected type=requirement, got %q", r.Type)
		}
	}
}

func TestSearchByStatus(t *testing.T) {
	s, _ := setupSearchDB(t)
	results, err := s.ByStatus("testproj", "active")
	if err != nil {
		t.Fatalf("ByStatus: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("ByStatus(active): got %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.Status != "active" {
			t.Errorf("ByStatus: expected status=active, got %q", r.Status)
		}
	}
}

func TestSearchByTag(t *testing.T) {
	s, _ := setupSearchDB(t)
	results, err := s.ByTag("testproj", "auth")
	if err != nil {
		t.Fatalf("ByTag: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("ByTag(auth): got %d results, want 2", len(results))
	}
}

func TestFullTextPrefersTitleOverBodyRepetition(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDatabase(dir + "/fts_rank.db")
	if err != nil {
		t.Fatalf("OpenDatabase: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ix := NewIndexer(db)

	bodyHeavy := []byte("pippi librarian readme repeated pippi librarian readme pippi librarian readme")
	titleHit := []byte("intro")

	for _, tc := range []struct {
		id    string
		title string
		body  []byte
	}{
		{"page-body", "Unrelated title", bodyHeavy},
		{"page-title", "DRAFT: pippi-librarian README", titleHit},
	} {
		fm := &schema.Frontmatter{
			ID:     tc.id,
			Title:  tc.title,
			Type:   "doc",
			Status: "draft",
		}
		if err := ix.IndexPage("p", "main", "pages/"+tc.id+".md", fm, tc.body); err != nil {
			t.Fatalf("IndexPage %s: %v", tc.id, err)
		}
	}

	s := NewSearcher(db)
	results, err := s.FullText("p", "pippi")
	if err != nil {
		t.Fatalf("FullText: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected 2 hits, got %d", len(results))
	}
	if results[0].ID != "page-title" {
		t.Fatalf("first result = %q (%s), want page-title (title match should beat body repetition)", results[0].ID, results[0].Title)
	}
}

func TestFullTextAllQueryTermsInTitleBeforeBodyOnly(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDatabase(dir + "/fts_terms.db")
	if err != nil {
		t.Fatalf("OpenDatabase: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ix := NewIndexer(db)

	pages := []struct {
		id    string
		title string
		body  []byte
	}{
		{"body-only", "Generic PLAN document", []byte("pippi readme pippi readme discussion")},
		{"title-hit", "DRAFT: pippi-librarian README.md", []byte("short")},
	}
	for _, p := range pages {
		fm := &schema.Frontmatter{ID: p.id, Title: p.title, Type: "doc", Status: "draft"}
		if err := ix.IndexPage("p", "main", "pages/"+p.id+".md", fm, p.body); err != nil {
			t.Fatalf("IndexPage: %v", err)
		}
	}

	s := NewSearcher(db)
	results, err := s.FullText("p", "pippi readme")
	if err != nil {
		t.Fatalf("FullText: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("want 2 hits, got %d", len(results))
	}
	if results[0].ID != "title-hit" {
		t.Fatalf("first = %q (%q), want title-hit (all query terms in title)", results[0].ID, results[0].Title)
	}
}

func TestSanitizeFTSQuery(t *testing.T) {
	// Apostrophe triggers FTS5 syntax error if passed raw to MATCH.
	if g, want := sanitizeFTSQuery("Anthropic's str_replace_editor"), "anthropic str replace editor"; g != want {
		t.Fatalf("sanitize apostrophe/underscore: got %q want %q", g, want)
	}
	if g := sanitizeFTSQuery("pippi-readme"); g != "pippi readme" {
		t.Fatalf("hyphen: got %q", g)
	}
	if sanitizeFTSQuery("a b") != "" {
		t.Fatalf("single-letter tokens dropped: got %q", sanitizeFTSQuery("a b"))
	}
}

func TestDependentsOf(t *testing.T) {
	s, ix := setupSearchDB(t)

	// Re-index P-001 with a provenance source so it depends on "ext-ref-001"
	fm := &schema.Frontmatter{
		ID:     "P-001",
		Title:  "Auth Requirement",
		Type:   "requirement",
		Status: "active",
		Tags:   []string{"auth", "security"},
		Provenance: schema.Provenance{
			Sources: []schema.Source{
				{Ref: "ext-ref-001", Version: 1, Status: "valid"},
			},
		},
	}
	if err := ix.IndexPage("testproj", "main", "pages/P-001.md", fm, []byte("body")); err != nil {
		t.Fatalf("IndexPage P-001 with provenance: %v", err)
	}

	results, err := s.DependentsOf("ext-ref-001")
	if err != nil {
		t.Fatalf("DependentsOf: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("DependentsOf(ext-ref-001): got %d results, want 1", len(results))
	}
	if len(results) > 0 && results[0].ID != "P-001" {
		t.Errorf("DependentsOf: expected P-001, got %q", results[0].ID)
	}
}

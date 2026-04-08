package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frodex/prd2wiki/internal/embedder"
	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/schema"
	"github.com/frodex/prd2wiki/internal/vectordb"
	"github.com/frodex/prd2wiki/internal/vocabulary"
)

func setupTestHandler(t *testing.T) (*Handler, http.Handler) {
	t.Helper()
	tmp := t.TempDir()

	repo, err := wgit.InitRepo(tmp, "test-project")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}

	dbPath := filepath.Join(tmp, "index.db")
	db, err := index.OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	repos := map[string]*wgit.Repo{"test-project": repo}

	indexr := index.NewIndexer(db)
	emb := embedder.ZeroEmbedder{Dims: 768}
	vstore := vectordb.NewStore(emb)
	vocab := vocabulary.NewStore(db)
	lib := librarian.New(repo, indexr, vstore, vocab)
	librarians := map[string]*librarian.Librarian{"test-project": lib}

	h := NewHandler(repos, db, librarians)

	mux := http.NewServeMux()
	h.Register(mux)

	return h, mux
}

func TestHome(t *testing.T) {
	_, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Projects") {
		t.Errorf("expected home page to contain 'Projects', got: %s", body[:200])
	}
}

func TestListPages(t *testing.T) {
	_, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/projects/test-project/pages", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "prd2wiki") {
		t.Error("response should contain 'prd2wiki' from layout")
	}
	if !strings.Contains(body, "test-project") {
		t.Error("response should contain the project name")
	}
	// Empty project should show "No pages yet" message.
	if !strings.Contains(body, "No pages yet") {
		t.Error("empty project should show 'No pages yet' message")
	}
}

func TestViewPageOnNonDefaultBranch(t *testing.T) {
	_, mux := setupTestHandler(t)

	// Write the page directly into the repo on a non-default branch (draft/agent).
	// setupTestHandler creates the repo internally, so we need our own setup here.
	tmp := t.TempDir()

	repo, err := wgit.InitRepo(tmp, "test-project")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}

	dbPath := filepath.Join(tmp, "index.db")
	db, err := index.OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	repos := map[string]*wgit.Repo{"test-project": repo}
	indexr := index.NewIndexer(db)
	emb := embedder.ZeroEmbedder{Dims: 768}
	vstore := vectordb.NewStore(emb)
	vocab := vocabulary.NewStore(db)
	lib := librarian.New(repo, indexr, vstore, vocab)
	librarians := map[string]*librarian.Librarian{"test-project": lib}

	h := NewHandler(repos, db, librarians)
	localMux := http.NewServeMux()
	h.Register(localMux)
	_ = mux // use local mux instead

	// Write a page on the "draft/agent" branch (not "truth" or "draft/incoming").
	fm := &schema.Frontmatter{
		ID:    "agent-page-001",
		Title: "Agent Page",
		Type:  "concept",
	}
	if err := repo.WritePageWithMeta("draft/agent", "pages/agent-page-001.md", fm, []byte("# Agent Page\n\nCreated by MCP.\n"), "add page", "test"); err != nil {
		t.Fatalf("WritePageWithMeta: %v", err)
	}

	// GET the page via the web UI — must return 200, not 404.
	req := httptest.NewRequest(http.MethodGet, "/projects/test-project/pages/agent-page-001", nil)
	rec := httptest.NewRecorder()
	localMux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("viewPage on draft/agent branch: expected 200, got %d (page was not found on non-default branch)", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Agent Page") {
		t.Errorf("response should contain page title 'Agent Page', got: %.200s", body)
	}
}

func TestStaticFiles(t *testing.T) {
	_, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for static CSS, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "wiki-nav") {
		t.Error("style.css should contain wiki-nav class")
	}
}

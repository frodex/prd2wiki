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
	emb := embedder.NewNoopEmbedder(768)
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

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/projects/default/pages" {
		t.Errorf("expected redirect to /projects/default/pages, got %s", loc)
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

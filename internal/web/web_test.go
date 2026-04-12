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

	h := NewHandler(repos, db, librarians, nil, nil)

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

	h := NewHandler(repos, db, librarians, nil, nil)
	localMux := http.NewServeMux()
	h.Register(localMux)
	_ = mux // use local mux instead

	// Write a page on the "draft/agent" branch (not "truth" or "draft/incoming").
	fm := &schema.Frontmatter{
		ID:    "agent-page-001",
		Title: "Agent Page",
		Type:  "concept",
	}
	if _, err := repo.WritePageWithMeta("draft/agent", "pages/agent-page-001.md", fm, []byte("# Agent Page\n\nCreated by MCP.\n"), "add page", "test"); err != nil {
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

func TestPathToTree(t *testing.T) {
	tests := []struct {
		path     string
		wantDirs []string
		wantFile string
	}{
		{"pages/DESIGN-003.md", nil, "DESIGN-003.md"},
		{"pages/docs/research/mechlab.md", []string{"docs", "research"}, "mechlab.md"},
		{"pages/docs/plans/roadmap.md", []string{"docs", "plans"}, "roadmap.md"},
		{"pages/core/auth.md", []string{"core"}, "auth.md"},
	}
	for _, tt := range tests {
		dirs, file := pathToTree(tt.path)
		if file != tt.wantFile {
			t.Errorf("pathToTree(%q) file = %q, want %q", tt.path, file, tt.wantFile)
		}
		if len(dirs) != len(tt.wantDirs) {
			t.Errorf("pathToTree(%q) dirs = %v, want %v", tt.path, dirs, tt.wantDirs)
			continue
		}
		for i := range dirs {
			if dirs[i] != tt.wantDirs[i] {
				t.Errorf("pathToTree(%q) dirs[%d] = %q, want %q", tt.path, i, dirs[i], tt.wantDirs[i])
			}
		}
	}
}

func TestBuildTreeFromPaths(t *testing.T) {
	items := []PageListItem{
		{ID: "flat-1", Path: "pages/flat-1.md"},
		{ID: "flat-2", Path: "pages/flat-2.md"},
		{ID: "mechlab", Path: "pages/docs/research/mechlab.md"},
		{ID: "roadmap", Path: "pages/docs/plans/roadmap.md"},
		{ID: "auth", Path: "pages/core/auth.md"},
		{ID: "login", Path: "pages/core/login.md"},
	}

	tree := buildTree(items, "")

	// First node is always "All" with total count.
	if tree[0].Name != "All" || tree[0].Count != 6 {
		t.Errorf("All node: got name=%q count=%d, want name=All count=6", tree[0].Name, tree[0].Count)
	}

	// Should have "core" and "docs" top-level directories (flat pages don't create nodes).
	dirNames := map[string]bool{}
	for _, node := range tree[1:] {
		dirNames[node.Name] = true
	}
	if !dirNames["core"] {
		t.Error("expected 'core' directory node")
	}
	if !dirNames["docs"] {
		t.Error("expected 'docs' directory node")
	}

	// "core" should have count 2.
	for _, node := range tree[1:] {
		if node.Name == "core" && node.Count != 2 {
			t.Errorf("core count: got %d, want 2", node.Count)
		}
		if node.Name == "docs" {
			if node.Count != 2 {
				t.Errorf("docs count: got %d, want 2", node.Count)
			}
			// "docs" should have children "plans" and "research".
			childNames := map[string]bool{}
			for _, child := range node.Children {
				childNames[child.Name] = true
			}
			if !childNames["research"] || !childNames["plans"] {
				t.Errorf("docs children: got %v, want research and plans", childNames)
			}
		}
	}

	// Test active filter.
	tree2 := buildTree(items, "core")
	for _, node := range tree2[1:] {
		if node.Name == "core" && !node.Active {
			t.Error("core should be active when filter is 'core'")
		}
	}
}

func TestStaticFiles(t *testing.T) {
	_, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/static/css/base.css", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for static CSS, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "wiki-nav") {
		t.Error("base.css should contain wiki-nav class")
	}
}

func TestAdminIndex(t *testing.T) {
	_, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Export") || !strings.Contains(body, "/admin/export") {
		snippet := body
		if len(snippet) > 400 {
			snippet = snippet[:400] + "…"
		}
		t.Errorf("admin index should link to export, got: %s", snippet)
	}
}

func TestAdminPOSTRequiresConfiguredKeys(t *testing.T) {
	_, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/admin/export", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when key store nil, got %d: %s", rec.Code, rec.Body.String())
	}
}

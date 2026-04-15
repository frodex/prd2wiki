package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/frodex/prd2wiki/internal/auth"
	"github.com/frodex/prd2wiki/internal/embedder"
	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/tree"
	"github.com/frodex/prd2wiki/internal/vectordb"
	"github.com/frodex/prd2wiki/internal/vocabulary"
	"github.com/frodex/prd2wiki/internal/web"
	"os"
)

// setupAuthServer creates a server with Keys configured but no tree index.
// Tree API auth rejection tests only need Keys; the 401 fires before tree lookup.
func setupAuthServer(t *testing.T) (*Server, string) {
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

	keyStore, err := auth.NewServiceKeyStore(db)
	if err != nil {
		t.Fatalf("key store: %v", err)
	}
	_, rawKey, err := keyStore.Issue(context.Background(), "test@prd2wiki", []string{"read", "write"}, 0, false)
	if err != nil {
		t.Fatalf("issue key: %v", err)
	}

	repos := map[string]*wgit.Repo{"test-project": repo}
	indexr := index.NewIndexer(db)
	vstore := vectordb.NewStore(embedder.ZeroEmbedder{Dims: 768})
	vocab := vocabulary.NewStore(db)
	lib := librarian.New(repo, indexr, vstore, vocab)

	// Build tree index with symlink structure the scanner expects.
	treeRoot := filepath.Join(tmp, "tree")
	projUUID := "deadbeef-1234-5678-abcd-000000000001"
	if err := tree.WriteProjectUUIDFile(treeRoot, "test-project", projUUID, "test-project"); err != nil {
		t.Fatalf("write project uuid: %v", err)
	}
	// Scanner needs repos/proj_{prefix}.git symlink to map UUID → RepoKey.
	reposDir := filepath.Join(tmp, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatalf("mkdir repos: %v", err)
	}
	repoSrc := filepath.Join(tmp, "test-project.wiki.git")
	linkDst := filepath.Join(reposDir, "proj_deadbeef.git")
	if err := os.Symlink(repoSrc, linkDst); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	idx, err := tree.Scan(treeRoot, tmp)
	if err != nil {
		t.Fatalf("tree scan: %v", err)
	}
	treeHolder := tree.NewIndexHolder(treeRoot, tmp, idx)

	srv := NewServer(ServerConfig{
		Addr:       ":0",
		Repos:      repos,
		DB:         db,
		Librarians: map[string]*librarian.Librarian{"test-project": lib},
		Edits:      map[string]*web.EditCache{"test-project": web.NewEditCache()},
		Keys:       keyStore,
		Tree:       treeHolder,
	})
	return srv, rawKey
}

// TestTreeMutationRequiresAuth verifies tree POST/PUT/DELETE return 401 without Bearer.
func TestTreeMutationRequiresAuth(t *testing.T) {
	srv, _ := setupAuthServer(t)
	handler := srv.Handler()

	body, _ := json.Marshal(TreeCreateRequest{Title: "Test", Body: "# T"})

	tests := []struct {
		method string
		path   string
		body   []byte
	}{
		{http.MethodPost, "/api/tree/test-project/pages", body},
		{http.MethodPut, "/api/tree/test-project/some-slug", body},
		{http.MethodDelete, "/api/tree/test-project/some-slug", nil},
	}

	for _, tc := range tests {
		var reader *bytes.Reader
		if tc.body != nil {
			reader = bytes.NewReader(tc.body)
		}
		var req *http.Request
		if reader != nil {
			req = httptest.NewRequest(tc.method, tc.path, reader)
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(tc.method, tc.path, nil)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without auth: expected 401, got %d: %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

// TestTreeCreateWithValidAuth verifies tree POST succeeds with a valid Bearer token.
func TestTreeCreateWithValidAuth(t *testing.T) {
	srv, token := setupAuthServer(t)
	handler := srv.Handler()

	body, _ := json.Marshal(TreeCreateRequest{
		Title: "Auth Test Page",
		Body:  "# Test content",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/tree/test-project/pages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("tree POST with auth: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["title"] != "Auth Test Page" {
		t.Errorf("title: got %v, want Auth Test Page", resp["title"])
	}
}

// TestInvalidTokenReturns401 verifies that a bad token is rejected.
func TestInvalidTokenReturns401(t *testing.T) {
	srv, _ := setupAuthServer(t)
	handler := srv.Handler()

	body, _ := json.Marshal(TreeCreateRequest{Title: "Bad", Body: "# Bad"})
	req := httptest.NewRequest(http.MethodPost, "/api/tree/test-project/pages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer psk_totally_invalid_key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token: expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestProjectMutationNoAuthToday documents that project API mutations currently
// succeed without auth. Phase 2 will change these to require auth — update this
// test to expect 401 when enforcement is added.
func TestProjectMutationNoAuthToday(t *testing.T) {
	srv, _ := setupAuthServer(t)
	handler := srv.Handler()

	body, _ := json.Marshal(CreatePageRequest{
		ID: "noauth-001", Title: "No Auth", Type: "concept", Body: "# Test",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/projects/test-project/pages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("project POST without auth: expected 201 (pre-Phase-2), got %d: %s", rec.Code, rec.Body.String())
	}
}

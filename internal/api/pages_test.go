package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/frodex/prd2wiki/internal/embedder"
	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/vectordb"
	"github.com/frodex/prd2wiki/internal/vocabulary"
	"github.com/frodex/prd2wiki/internal/web"
)

func setupTestServer(t *testing.T) *Server {
	t.Helper()
	tmp := t.TempDir()

	// Init a git bare repo for "test-project".
	repo, err := wgit.InitRepo(tmp, "test-project")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}

	// Open SQLite database.
	dbPath := filepath.Join(tmp, "index.db")
	db, err := index.OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	repos := map[string]*wgit.Repo{
		"test-project": repo,
	}

	indexer := index.NewIndexer(db)
	emb := embedder.ZeroEmbedder{Dims: 768}
	vstore := vectordb.NewStore(emb)
	vocab := vocabulary.NewStore(db)
	lib := librarian.New(repo, indexer, vstore, vocab)
	librarians := map[string]*librarian.Librarian{"test-project": lib}
	edits := map[string]*web.EditCache{"test-project": web.NewEditCache()}

	return NewServer(ServerConfig{
		Addr:       ":0",
		Repos:      repos,
		DB:         db,
		Librarians: librarians,
		Edits:      edits,
	})
}

func TestCreateAndGetPage(t *testing.T) {
	srv := setupTestServer(t)
	handler := srv.Handler()

	// POST to create a page.
	body := CreatePageRequest{
		ID:     "req-001",
		Title:  "First Requirement",
		Type:   "requirement",
		Status: "draft",
		Body:   "# Hello\n\nThis is a test page.",
		Tags:   []string{"test", "demo"},
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/test-project/pages", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var createResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp["id"] != "req-001" {
		t.Errorf("create response id: got %v, want req-001", createResp["id"])
	}

	// GET to read it back.
	req = httptest.NewRequest(http.MethodGet, "/api/projects/test-project/pages/req-001?branch=draft/incoming", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var getResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if getResp["id"] != "req-001" {
		t.Errorf("get id: got %v, want req-001", getResp["id"])
	}
	if getResp["title"] != "First Requirement" {
		t.Errorf("get title: got %v, want First Requirement", getResp["title"])
	}
}

func TestCreatePageValidationError(t *testing.T) {
	srv := setupTestServer(t)
	handler := srv.Handler()

	// POST with missing required fields (no id, no title, no type).
	// Use conform intent so the librarian blocks on validation errors.
	body := CreatePageRequest{
		Body:   "some body",
		Intent: "conform",
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/test-project/pages", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("validation: expected 422, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["valid"] != false {
		t.Errorf("expected valid=false, got %v", resp["valid"])
	}
	issues, ok := resp["issues"].([]interface{})
	if !ok || len(issues) == 0 {
		t.Errorf("expected non-empty issues array, got %v", resp["issues"])
	}
}

func TestDeletePage(t *testing.T) {
	srv := setupTestServer(t)
	handler := srv.Handler()

	// POST to create a page.
	body := CreatePageRequest{
		ID:    "del-001",
		Title: "To Be Deleted",
		Type:  "concept",
		Body:  "Temporary page.",
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/test-project/pages", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// DELETE the page.
	req = httptest.NewRequest(http.MethodDelete, "/api/projects/test-project/pages/del-001?branch=draft/incoming", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// GET to verify 404.
	req = httptest.NewRequest(http.MethodGet, "/api/projects/test-project/pages/del-001?branch=draft/incoming", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListPages(t *testing.T) {
	srv := setupTestServer(t)
	handler := srv.Handler()

	// POST multiple pages with different types.
	pages := []CreatePageRequest{
		{ID: "req-a", Title: "Requirement A", Type: "requirement", Body: "body a"},
		{ID: "req-b", Title: "Requirement B", Type: "requirement", Body: "body b"},
		{ID: "con-a", Title: "Concept A", Type: "concept", Body: "body c"},
	}
	for _, p := range pages {
		bodyJSON, _ := json.Marshal(p)
		req := httptest.NewRequest(http.MethodPost, "/api/projects/test-project/pages", bytes.NewReader(bodyJSON))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s: expected 201, got %d: %s", p.ID, rec.Code, rec.Body.String())
		}
	}

	// GET list with type filter.
	req := httptest.NewRequest(http.MethodGet, "/api/projects/test-project/pages?type=requirement", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 requirements, got %d", len(results))
	}

	// GET list with no filter — should return all 3.
	req = httptest.NewRequest(http.MethodGet, "/api/projects/test-project/pages", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list all: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode list all response: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 pages, got %d", len(results))
	}

}

func TestProjectNotFound(t *testing.T) {
	srv := setupTestServer(t)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/projects/nonexistent/pages", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown project, got %d", rec.Code)
	}
}

func TestCreatePageFlatPayload(t *testing.T) {
	srv := setupTestServer(t)
	handler := srv.Handler()

	// POST with flat JSON matching what the browser sends — this was the bug format.
	rawJSON := []byte(`{"id":"TEST-FLAT","title":"Flat Payload Test","type":"concept","body":"# Test","intent":"verbatim"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/projects/test-project/pages", bytes.NewReader(rawJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var createResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if createResp["id"] != "TEST-FLAT" {
		t.Errorf("id: got %v, want TEST-FLAT", createResp["id"])
	}
	if createResp["title"] != "Flat Payload Test" {
		t.Errorf("title: got %v, want 'Flat Payload Test' — flat payload was not parsed correctly", createResp["title"])
	}
}

func TestCreatePageNoID(t *testing.T) {
	srv := setupTestServer(t)
	handler := srv.Handler()

	// POST with title but no ID — server must auto-generate one.
	rawJSON := []byte(`{"title":"Auto ID Page","type":"concept","body":"# Auto ID","intent":"verbatim"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/projects/test-project/pages", bytes.NewReader(rawJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var createResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	generatedID, ok := createResp["id"].(string)
	if !ok || generatedID == "" {
		t.Fatalf("expected a non-empty generated id in response, got %v", createResp["id"])
	}

	// GET the page using the returned ID — must exist.
	getURL := "/api/projects/test-project/pages/" + generatedID + "?branch=draft/incoming"
	req = httptest.NewRequest(http.MethodGet, getURL, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("get generated page: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var getResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if getResp["id"] != generatedID {
		t.Errorf("retrieved page id: got %v, want %v", getResp["id"], generatedID)
	}
}

func TestCreatePageWithIntents(t *testing.T) {
	srv := setupTestServer(t)

	// Test conform intent normalizes tags.
	body := map[string]interface{}{
		"id":     "INT-001",
		"title":  "Intent Test",
		"type":   "concept",
		"tags":   []string{"AUTH", "Security"},
		"body":   "# Content",
		"intent": "conform",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/projects/test-project/pages", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("conform POST status = %d, body = %s", w.Code, w.Body.String())
	}

	// Read back and verify tags are lowercase.
	req = httptest.NewRequest("GET", "/api/projects/test-project/pages/INT-001?branch=draft/incoming", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	tags := resp["tags"].([]interface{})
	if tags[0].(string) != "auth" {
		t.Errorf("conform should normalize tags to lowercase, got %v", tags)
	}
}

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
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

	return NewServer(":0", repos, db)
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
	body := CreatePageRequest{
		Body: "some body",
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

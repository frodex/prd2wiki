package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/schema"
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
	vocab := vocabulary.NewStore(db)
	lib := librarian.New(repo, indexer, vocab)
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

// ---------------------------------------------------------------------------
// §pre-flight-item-2 tests (T0-NEW-A / R13-6 — server-side frontmatter merge)
// ---------------------------------------------------------------------------
//
// These tests exercise the read-modify-write merge semantics on update:
//
//   - nil field in request = preserve existing
//   - [] (non-nil empty slice) = explicit clear
//   - null JSON value = treated as nil (preserve; Go json.Unmarshal cannot
//     distinguish null from absent for slices)
//   - dc.created preserved across update (backfilled to now when missing)
//   - dc.modified populated on every write (create and update)
//   - create path behavior unchanged (defaults still apply)
//
// Frontmatter assertions read via repo.ReadPageWithMeta since the GET handler
// does not currently surface dc.creator / dc.created / dc.modified in its
// JSON response shape.

// pageFMFromRepo reads the stored frontmatter for a page id via the server's
// resolvePagePath helper and ReadPageWithMeta — bypasses the GET handler so
// tests can assert on dc.* fields that the handler does not expose.
func pageFMFromRepo(t *testing.T, srv *Server, project, id, branch string) *schema.Frontmatter {
	t.Helper()
	repo := srv.repos[project]
	if repo == nil {
		t.Fatalf("no repo for project %q", project)
	}
	path := srv.resolvePagePath(project, id)
	fm, _, err := repo.ReadPageWithMeta(branch, path)
	if err != nil {
		t.Fatalf("ReadPageWithMeta(%s, %s): %v", branch, path, err)
	}
	if fm == nil {
		t.Fatalf("nil frontmatter for %s/%s", project, id)
	}
	return fm
}

// postJSON issues a POST /api/projects/test-project/pages with the given JSON
// body and returns the parsed response map. Fails the test if status != 201.
func postJSON(t *testing.T, srv *Server, body []byte) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/projects/test-project/pages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode POST resp: %v", err)
	}
	return resp
}

// putJSON issues a PUT /api/projects/test-project/pages/{id} with the given
// raw JSON body. Returns status code and response body. Does not fail on
// non-200 so tests can assert on status.
func putJSON(t *testing.T, srv *Server, id string, body []byte) (int, []byte) {
	t.Helper()
	url := "/api/projects/test-project/pages/" + id + "?branch=draft/incoming"
	req := httptest.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

// getJSON issues a GET /api/projects/test-project/pages/{id}?branch=... and
// returns the parsed response map.
func getJSON(t *testing.T, srv *Server, id string) map[string]interface{} {
	t.Helper()
	url := "/api/projects/test-project/pages/" + id + "?branch=draft/incoming"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: expected 200, got %d: %s", url, rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode GET resp: %v", err)
	}
	return resp
}

// tagsEqual compares two string slices for equality.
func tagsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestUpdatePartialBodyPreservesFrontmatter (T2.1) — POST with full
// frontmatter, PUT with {id, title, body} only, verify type/status/tags/
// dc.creator/dc.created preserved and dc.modified populated.
func TestUpdatePartialBodyPreservesFrontmatter(t *testing.T) {
	srv := setupTestServer(t)

	postBody, _ := json.Marshal(CreatePageRequest{
		ID:     "pf-001",
		Title:  "Partial PUT Preserve",
		Type:   "research",
		Status: "superseded",
		Tags:   []string{"a", "b", "c"},
		Body:   "# t0",
		Author: "creator@example.com",
	})
	postJSON(t, srv, postBody)

	fmPre := pageFMFromRepo(t, srv, "test-project", "pf-001", "draft/incoming")
	if fmPre.Type != "research" || fmPre.Status != "superseded" {
		t.Fatalf("POST fm: type=%q status=%q; want research/superseded", fmPre.Type, fmPre.Status)
	}
	if !tagsEqual(fmPre.Tags, []string{"a", "b", "c"}) {
		t.Fatalf("POST fm.Tags: %v; want [a b c]", fmPre.Tags)
	}
	if fmPre.DCCreator != "creator@example.com" {
		t.Fatalf("POST fm.DCCreator: %q; want creator@example.com", fmPre.DCCreator)
	}

	// Partial PUT: only id, title, body. Omit type/status/tags/author.
	putBody := []byte(`{"id":"pf-001","title":"Partial PUT Preserve","body":"# t1"}`)
	code, respBody := putJSON(t, srv, "pf-001", putBody)
	if code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d: %s", code, respBody)
	}

	fmPost := pageFMFromRepo(t, srv, "test-project", "pf-001", "draft/incoming")
	if fmPost.Type != "research" {
		t.Errorf("PUT fm.Type: got %q, want research (preserved)", fmPost.Type)
	}
	if fmPost.Status != "superseded" {
		t.Errorf("PUT fm.Status: got %q, want superseded (preserved)", fmPost.Status)
	}
	if !tagsEqual(fmPost.Tags, []string{"a", "b", "c"}) {
		t.Errorf("PUT fm.Tags: got %v, want [a b c] (preserved)", fmPost.Tags)
	}
	if fmPost.DCCreator != "creator@example.com" {
		t.Errorf("PUT fm.DCCreator: got %q, want creator@example.com (preserved)", fmPost.DCCreator)
	}
	if fmPost.DCCreated.Time.IsZero() {
		t.Errorf("PUT fm.DCCreated: zero; want preserved from POST")
	}
	if fmPost.DCModified.Time.IsZero() {
		t.Errorf("PUT fm.DCModified: zero; want populated (non-zero) on every write")
	}
	if fmPost.DCModified.Time.Before(fmPost.DCCreated.Time) {
		t.Errorf("PUT fm.DCModified (%v) < DCCreated (%v); want dc.modified >= dc.created",
			fmPost.DCModified.Time, fmPost.DCCreated.Time)
	}
}

// TestUpdateExplicitEmptyTagsClears (T2.2) — POST with tags=[x,y,z], PUT with
// tags=[] (non-nil empty), verify tags are cleared.
func TestUpdateExplicitEmptyTagsClears(t *testing.T) {
	srv := setupTestServer(t)

	postBody, _ := json.Marshal(CreatePageRequest{
		ID: "pf-002", Title: "Clear Tags Test", Type: "concept", Status: "draft",
		Tags: []string{"x", "y", "z"}, Body: "# body",
	})
	postJSON(t, srv, postBody)

	// Explicit empty tags: non-nil empty slice → clear.
	putBody := []byte(`{"id":"pf-002","title":"Clear Tags Test","type":"concept","status":"draft","tags":[],"body":"# body"}`)
	code, respBody := putJSON(t, srv, "pf-002", putBody)
	if code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d: %s", code, respBody)
	}

	fmPost := pageFMFromRepo(t, srv, "test-project", "pf-002", "draft/incoming")
	if len(fmPost.Tags) != 0 {
		t.Errorf("fm.Tags after explicit-clear PUT: %v; want empty/nil (cleared)", fmPost.Tags)
	}
}

// TestUpdateFullFrontmatterNoRegression (T2.3) — POST with one set of
// frontmatter, seed a past dc.created, then PUT with a completely different
// set. Verify PUT frontmatter wins, dc.created is preserved (past date), and
// dc.modified is populated. Past-date seed makes preserve-vs-reset on
// dc.created distinguishable at YAML day-precision.
func TestUpdateFullFrontmatterNoRegression(t *testing.T) {
	srv := setupTestServer(t)

	postBody, _ := json.Marshal(CreatePageRequest{
		ID: "pf-003", Title: "Full Replace", Type: "research", Status: "draft",
		Tags: []string{"old1", "old2"}, Body: "# body", Author: "original@example.com",
	})
	postJSON(t, srv, postBody)

	// Seed past dc.created to distinguish preserve-vs-reset.
	repo := srv.repos["test-project"]
	pgPath := srv.resolvePagePath("test-project", "pf-003")
	fm, body, err := repo.ReadPageWithMeta("draft/incoming", pgPath)
	if err != nil {
		t.Fatalf("ReadPageWithMeta: %v", err)
	}
	past := time.Now().UTC().Add(-72 * time.Hour)
	fm.DCCreated = schema.Date{Time: past}
	if _, err := repo.WritePageWithMeta("draft/incoming", pgPath, fm, body, "seed past dc.created", "test"); err != nil {
		t.Fatalf("WritePageWithMeta: %v", err)
	}

	putBody, _ := json.Marshal(CreatePageRequest{
		ID: "pf-003", Title: "Full Replace", Type: "spec", Status: "approved",
		Tags: []string{"d", "e"}, Body: "# body", Author: "updater@example.com",
	})
	code, respBody := putJSON(t, srv, "pf-003", putBody)
	if code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d: %s", code, respBody)
	}

	fmPost := pageFMFromRepo(t, srv, "test-project", "pf-003", "draft/incoming")
	if fmPost.Type != "spec" {
		t.Errorf("fm.Type: got %q, want spec", fmPost.Type)
	}
	if fmPost.Status != "approved" {
		t.Errorf("fm.Status: got %q, want approved", fmPost.Status)
	}
	if !tagsEqual(fmPost.Tags, []string{"d", "e"}) {
		t.Errorf("fm.Tags: got %v, want [d e]", fmPost.Tags)
	}
	if fmPost.DCCreator != "updater@example.com" {
		t.Errorf("fm.DCCreator: got %q, want updater@example.com", fmPost.DCCreator)
	}
	// dc.created preserved from the seeded past date (pre-fix handler would
	// have reset this to today; post-fix preserves existing).
	if !fmPost.DCCreated.Time.Equal(past.Truncate(24 * time.Hour)) {
		t.Errorf("fm.DCCreated: got %v, want %v (preserved from seeded past date)",
			fmPost.DCCreated.Time, past.Truncate(24*time.Hour))
	}
	// dc.modified populated (non-zero) — pre-fix handler never set this field.
	if fmPost.DCModified.Time.IsZero() {
		t.Errorf("fm.DCModified: zero; want populated on every write (including full-replace PUT)")
	}
}

// TestUpdateRoundTripIntegrity (T2.5) — POST, seed past dc.created, read
// snapshot 1 via repo, PUT same body+frontmatter, read snapshot 2 via repo.
// Snapshots must be identical except dc.modified (which the fix updates on
// every write). Past-date seed on dc.created makes preserve-vs-reset
// distinguishable at YAML day-precision.
func TestUpdateRoundTripIntegrity(t *testing.T) {
	srv := setupTestServer(t)

	postBody, _ := json.Marshal(CreatePageRequest{
		ID: "pf-005", Title: "Round Trip", Type: "concept", Status: "draft",
		Tags: []string{"rt"}, Body: "# rt", Author: "rt@example.com",
	})
	postJSON(t, srv, postBody)

	// Seed past dc.created.
	repo := srv.repos["test-project"]
	pgPath := srv.resolvePagePath("test-project", "pf-005")
	fm, body, err := repo.ReadPageWithMeta("draft/incoming", pgPath)
	if err != nil {
		t.Fatalf("ReadPageWithMeta: %v", err)
	}
	past := time.Now().UTC().Add(-72 * time.Hour)
	fm.DCCreated = schema.Date{Time: past}
	if _, err := repo.WritePageWithMeta("draft/incoming", pgPath, fm, body, "seed past dc.created", "test"); err != nil {
		t.Fatalf("WritePageWithMeta: %v", err)
	}

	snap1 := pageFMFromRepo(t, srv, "test-project", "pf-005", "draft/incoming")

	// PUT with same frontmatter + body.
	code, respBody := putJSON(t, srv, "pf-005", postBody)
	if code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d: %s", code, respBody)
	}

	snap2 := pageFMFromRepo(t, srv, "test-project", "pf-005", "draft/incoming")

	// Compare: all fields except dc.modified must match.
	if snap1.ID != snap2.ID || snap1.Title != snap2.Title || snap1.Type != snap2.Type || snap1.Status != snap2.Status {
		t.Errorf("core fm fields differ across POST+seed -> PUT(same) -> read:\n  snap1=%+v\n  snap2=%+v", snap1, snap2)
	}
	if !tagsEqual(snap1.Tags, snap2.Tags) {
		t.Errorf("fm.Tags differ across round-trip: snap1=%v snap2=%v", snap1.Tags, snap2.Tags)
	}
	if snap1.DCCreator != snap2.DCCreator {
		t.Errorf("fm.DCCreator differ: snap1=%q snap2=%q", snap1.DCCreator, snap2.DCCreator)
	}
	// dc.created: preserved (pre-fix would have reset to today; post-fix
	// preserves the seeded past value).
	if !snap1.DCCreated.Time.Equal(snap2.DCCreated.Time) {
		t.Errorf("fm.DCCreated differs across round-trip: snap1=%v snap2=%v (want preserved)", snap1.DCCreated.Time, snap2.DCCreated.Time)
	}
}

// TestDCCreatedPreservedDCModifiedUpdated (T2.6) — seed a page with
// dc.created set to 3 days ago (directly via the repo), then PUT partial
// via HTTP. Verify dc.created is preserved (past date) and dc.modified is
// populated (today).
//
// This is the distinguishing test for preserve-vs-reset on dc.created.
// Same-day POST+PUT cannot distinguish at day-precision YAML; a past
// dc.created does.
func TestDCCreatedPreservedDCModifiedUpdated(t *testing.T) {
	srv := setupTestServer(t)

	postBody, _ := json.Marshal(CreatePageRequest{
		ID: "pf-006", Title: "DC Dates", Type: "concept", Status: "draft",
		Tags: []string{"d"}, Body: "# body", Author: "a@b",
	})
	postJSON(t, srv, postBody)

	// Directly rewrite the page with a past dc.created to create a
	// distinguishable preserve-vs-reset signal at YAML day-precision.
	repo := srv.repos["test-project"]
	path := srv.resolvePagePath("test-project", "pf-006")
	fm, body, err := repo.ReadPageWithMeta("draft/incoming", path)
	if err != nil {
		t.Fatalf("ReadPageWithMeta: %v", err)
	}
	past := time.Now().UTC().Add(-72 * time.Hour)
	fm.DCCreated = schema.Date{Time: past}
	if _, err := repo.WritePageWithMeta("draft/incoming", path, fm, body, "seed past dc.created", "test"); err != nil {
		t.Fatalf("WritePageWithMeta: %v", err)
	}

	// Sanity: confirm the seeded date stuck.
	fmPre := pageFMFromRepo(t, srv, "test-project", "pf-006", "draft/incoming")
	if fmPre.DCCreated.Time.After(past.Add(24 * time.Hour)) {
		t.Fatalf("seed failed: fm.DCCreated=%v; wanted around %v", fmPre.DCCreated.Time, past)
	}

	// Partial PUT.
	putBody := []byte(`{"id":"pf-006","title":"DC Dates","body":"# body v2"}`)
	code, respBody := putJSON(t, srv, "pf-006", putBody)
	if code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d: %s", code, respBody)
	}

	fmPost := pageFMFromRepo(t, srv, "test-project", "pf-006", "draft/incoming")
	// dc.created preserved — still the seeded past date (pre-fix handler
	// would have reset this to today).
	if !fmPost.DCCreated.Time.Equal(fmPre.DCCreated.Time) {
		t.Errorf("fm.DCCreated after partial PUT: got %v, want %v (preserved from seeded past date)",
			fmPost.DCCreated.Time, fmPre.DCCreated.Time)
	}
	// dc.modified populated (non-zero) — pre-fix handler never set this field.
	if fmPost.DCModified.Time.IsZero() {
		t.Errorf("fm.DCModified after PUT: zero; want populated on every write")
	}
	// And dc.modified > dc.created (today > 3 days ago).
	if !fmPost.DCModified.Time.After(fmPost.DCCreated.Time) {
		t.Errorf("fm.DCModified (%v) must be After fm.DCCreated (%v) for partial PUT on aged page",
			fmPost.DCModified.Time, fmPost.DCCreated.Time)
	}
}

// TestCreatePathDefaultsUnchanged (T2.8) — POST with only {title, body} on
// the create path. Response is 201; dc.created is populated (today) and
// status defaults to "draft". The fix must not regress the create path.
func TestCreatePathDefaultsUnchanged(t *testing.T) {
	srv := setupTestServer(t)

	// POST with minimal body — no id, no status, no tags, no type.
	rawJSON := []byte(`{"title":"Create Defaults","body":"# body","intent":"verbatim"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/projects/test-project/pages", bytes.NewReader(rawJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "draft" {
		t.Errorf("create resp status: got %v, want draft (default)", resp["status"])
	}

	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatalf("no id in create response")
	}
	fm := pageFMFromRepo(t, srv, "test-project", id, "draft/incoming")
	if fm.Status != "draft" {
		t.Errorf("fm.Status: got %q, want draft (default)", fm.Status)
	}
	if fm.DCCreated.Time.IsZero() {
		t.Errorf("fm.DCCreated: zero; want populated on create")
	}
	if fm.DCModified.Time.IsZero() {
		t.Errorf("fm.DCModified: zero; want populated on create (fix sets dc.modified on every write)")
	}
}

// TestExplicitNullTagsPreserves (T2.9) — POST with tags=[a,b], PUT with raw
// JSON containing `"tags": null`. Verify tags preserved. Go's json.Unmarshal
// cannot distinguish null from absent for slices (both → nil), so the merge
// contract treats null as "preserve existing". Documented behavior.
func TestExplicitNullTagsPreserves(t *testing.T) {
	srv := setupTestServer(t)

	postBody, _ := json.Marshal(CreatePageRequest{
		ID: "pf-009", Title: "Null Tags", Type: "concept", Status: "draft",
		Tags: []string{"a", "b"}, Body: "# body",
	})
	postJSON(t, srv, postBody)

	// Raw JSON with `"tags": null`.
	putBody := []byte(`{"id":"pf-009","title":"Null Tags","body":"# body v2","tags":null}`)
	code, respBody := putJSON(t, srv, "pf-009", putBody)
	if code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d: %s", code, respBody)
	}

	fmPost := pageFMFromRepo(t, srv, "test-project", "pf-009", "draft/incoming")
	if !tagsEqual(fmPost.Tags, []string{"a", "b"}) {
		t.Errorf("fm.Tags after PUT with \"tags\": null: got %v, want [a b] (null treated as preserve)", fmPost.Tags)
	}
}

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetReferences(t *testing.T) {
	srv := setupTestServer(t)
	handler := srv.Handler()

	// Create page P-001 with a provenance source "src-001".
	body := map[string]interface{}{
		"id":    "p-001",
		"title": "Page One",
		"type":  "requirement",
		"body":  "# Page One\n\nContent here.",
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/test-project/pages", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create P-001: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Insert a provenance edge directly into the database for P-001 -> SRC-001.
	_, err := srv.db.Exec(`
		INSERT INTO provenance_edges (source_page, target_ref, target_version, target_checksum, status)
		VALUES ('p-001', 'src-001', 1, 'abc123', 'valid')
	`)
	if err != nil {
		t.Fatalf("insert provenance edge: %v", err)
	}

	// GET references for P-001 with depth=1.
	req = httptest.NewRequest(http.MethodGet, "/api/projects/test-project/pages/P-001/references?depth=1", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("references: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var refResp RefNode
	if err := json.Unmarshal(rec.Body.Bytes(), &refResp); err != nil {
		t.Fatalf("decode references response: %v", err)
	}

	if refResp.Ref != "p-001" {
		t.Errorf("root ref: got %q, want p-001", refResp.Ref)
	}
	if refResp.Status != "root" {
		t.Errorf("root status: got %q, want root", refResp.Status)
	}
	if len(refResp.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(refResp.Children))
	}

	child := refResp.Children[0]
	if child.Ref != "src-001" {
		t.Errorf("child ref: got %q, want SRC-001", child.Ref)
	}
	if child.Status != "valid" {
		t.Errorf("child status: got %q, want valid", child.Status)
	}
	if child.Version != 1 {
		t.Errorf("child version: got %d, want 1", child.Version)
	}
	if child.Checksum != "abc123" {
		t.Errorf("child checksum: got %q, want abc123", child.Checksum)
	}
}

func TestGetReferencesProjectNotFound(t *testing.T) {
	srv := setupTestServer(t)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/projects/nonexistent/pages/P-001/references", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown project, got %d", rec.Code)
	}
}

func TestGetReferencesDepthClamped(t *testing.T) {
	srv := setupTestServer(t)
	handler := srv.Handler()

	// Create a page so the project is valid.
	body := map[string]interface{}{
		"id":    "P-002",
		"title": "Page Two",
		"type":  "concept",
		"body":  "Content.",
	}
	bodyJSON, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/projects/test-project/pages", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create P-002: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Request with depth > 5 should be clamped and still work.
	req = httptest.NewRequest(http.MethodGet, "/api/projects/test-project/pages/P-002/references?depth=10", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("references depth=10: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSearchPages(t *testing.T) {
	srv := setupTestServer(t)
	handler := srv.Handler()

	// Create a page.
	body := map[string]interface{}{
		"id":    "S-001",
		"title": "Searchable Page",
		"type":  "requirement",
		"body":  "Search content.",
	}
	bodyJSON, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/projects/test-project/pages", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create S-001: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Search with type filter.
	req = httptest.NewRequest(http.MethodGet, "/api/projects/test-project/search?type=requirement", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("search: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if len(results) < 1 {
		t.Errorf("expected at least 1 result, got %d", len(results))
	}

	// Search with no filters (falls back to ListAll).
	req = httptest.NewRequest(http.MethodGet, "/api/projects/test-project/search", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("search all: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

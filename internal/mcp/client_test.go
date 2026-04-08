package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient starts a mock HTTP server with the given handler and returns
// a WikiClient pointed at it, plus a cleanup function.
func newTestClient(t *testing.T, handler http.Handler) (*WikiClient, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	return NewWikiClient(srv.URL), srv.Close
}

// mustJSON marshals v to JSON or fails the test.
func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// ---------------------------------------------------------------------------
// GetPage
// ---------------------------------------------------------------------------

func TestGetPage(t *testing.T) {
	want := PageResponse{
		ID:         "req-001",
		Title:      "First Requirement",
		Type:       "requirement",
		Status:     "draft",
		TrustLevel: 1,
		Tags:       []string{"test", "demo"},
		Body:       "# Hello",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/myproj/pages/req-001", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("branch") != "draft/incoming" {
			t.Errorf("expected branch=draft/incoming, got %q", r.URL.Query().Get("branch"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	got, err := client.GetPage("myproj", "req-001", "draft/incoming")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}

	if got.ID != want.ID {
		t.Errorf("ID: got %q, want %q", got.ID, want.ID)
	}
	if got.Title != want.Title {
		t.Errorf("Title: got %q, want %q", got.Title, want.Title)
	}
	if got.Type != want.Type {
		t.Errorf("Type: got %q, want %q", got.Type, want.Type)
	}
	if got.Status != want.Status {
		t.Errorf("Status: got %q, want %q", got.Status, want.Status)
	}
	if got.TrustLevel != want.TrustLevel {
		t.Errorf("TrustLevel: got %d, want %d", got.TrustLevel, want.TrustLevel)
	}
	if len(got.Tags) != len(want.Tags) {
		t.Errorf("Tags len: got %d, want %d", len(got.Tags), len(want.Tags))
	}
	if got.Body != want.Body {
		t.Errorf("Body: got %q, want %q", got.Body, want.Body)
	}
}

func TestGetPageNoBranch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages/p1", func(w http.ResponseWriter, r *http.Request) {
		// branch query param must be absent when not specified
		if b := r.URL.Query().Get("branch"); b != "" {
			t.Errorf("expected no branch param, got %q", b)
		}
		json.NewEncoder(w).Encode(PageResponse{ID: "p1", Title: "P1"})
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	got, err := client.GetPage("proj", "p1", "")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.ID != "p1" {
		t.Errorf("ID: got %q, want p1", got.ID)
	}
}

func TestGetPageNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages/missing", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "page not found", http.StatusNotFound)
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	_, err := client.GetPage("proj", "missing", "")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

// ---------------------------------------------------------------------------
// CreatePage
// ---------------------------------------------------------------------------

func TestCreatePage(t *testing.T) {
	wantResp := CreatePageResponse{
		ID:     "new-001",
		Title:  "New Page",
		Status: "draft",
		Path:   "pages/new-001.md",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		var req CreatePageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad JSON", http.StatusBadRequest)
			return
		}
		if req.ID != "new-001" {
			t.Errorf("request ID: got %q, want new-001", req.ID)
		}
		if req.Title != "New Page" {
			t.Errorf("request Title: got %q, want New Page", req.Title)
		}
		if req.Type != "concept" {
			t.Errorf("request Type: got %q, want concept", req.Type)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(wantResp)
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	got, err := client.CreatePage("proj", CreatePageRequest{
		ID:    "new-001",
		Title: "New Page",
		Type:  "concept",
		Body:  "# Content",
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	if got.ID != wantResp.ID {
		t.Errorf("ID: got %q, want %q", got.ID, wantResp.ID)
	}
	if got.Title != wantResp.Title {
		t.Errorf("Title: got %q, want %q", got.Title, wantResp.Title)
	}
	if got.Status != wantResp.Status {
		t.Errorf("Status: got %q, want %q", got.Status, wantResp.Status)
	}
	if got.Path != wantResp.Path {
		t.Errorf("Path: got %q, want %q", got.Path, wantResp.Path)
	}
}

func TestCreatePageOptionalFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		var req CreatePageRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Branch != "feature/x" {
			t.Errorf("Branch: got %q, want feature/x", req.Branch)
		}
		if req.Intent != "conform" {
			t.Errorf("Intent: got %q, want conform", req.Intent)
		}
		if req.Author != "alice@example.com" {
			t.Errorf("Author: got %q, want alice@example.com", req.Author)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreatePageResponse{ID: "opt-001"})
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	_, err := client.CreatePage("proj", CreatePageRequest{
		ID:     "opt-001",
		Title:  "Optional Test",
		Type:   "requirement",
		Body:   "body",
		Branch: "feature/x",
		Intent: "conform",
		Author: "alice@example.com",
		Tags:   []string{"a", "b"},
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
}

func TestCreatePageServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "submit failed: internal error", http.StatusInternalServerError)
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	_, err := client.CreatePage("proj", CreatePageRequest{ID: "x", Title: "x", Type: "concept", Body: "x"})
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

// ---------------------------------------------------------------------------
// ListPages
// ---------------------------------------------------------------------------

func TestListPages(t *testing.T) {
	want := []PageResult{
		{ID: "req-a", Title: "Req A", Type: "requirement", Status: "draft", Project: "proj"},
		{ID: "req-b", Title: "Req B", Type: "requirement", Status: "draft", Project: "proj"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "requirement" {
			t.Errorf("expected type=requirement, got %q", r.URL.Query().Get("type"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	got, err := client.ListPages("proj", map[string]string{"type": "requirement"})
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].ID != "req-a" {
		t.Errorf("result[0].ID: got %q, want req-a", got[0].ID)
	}
	if got[1].ID != "req-b" {
		t.Errorf("result[1].ID: got %q, want req-b", got[1].ID)
	}
}

func TestListPagesNoFilters(t *testing.T) {
	all := []PageResult{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
		{ID: "c", Title: "C"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(all)
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	got, err := client.ListPages("proj", nil)
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 results, got %d", len(got))
	}
}

func TestListPagesEmpty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]PageResult{})
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	got, err := client.ListPages("proj", nil)
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d items", len(got))
	}
}

// ---------------------------------------------------------------------------
// DeletePage
// ---------------------------------------------------------------------------

func TestDeletePage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/projects/proj/pages/del-001", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("branch") != "draft/incoming" {
			t.Errorf("expected branch=draft/incoming, got %q", r.URL.Query().Get("branch"))
		}
		w.WriteHeader(http.StatusNoContent)
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	if err := client.DeletePage("proj", "del-001", "draft/incoming"); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}
}

func TestDeletePageNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/projects/proj/pages/missing", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "page not found", http.StatusNotFound)
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	if err := client.DeletePage("proj", "missing", ""); err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

func TestSearch(t *testing.T) {
	want := []PageResult{
		{ID: "s-001", Title: "Searchable", Type: "requirement", Status: "approved"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "requirement" {
			t.Errorf("expected type=requirement, got %q", r.URL.Query().Get("type"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	got, err := client.Search("proj", map[string]string{"type": "requirement"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].ID != "s-001" {
		t.Errorf("result ID: got %q, want s-001", got[0].ID)
	}
	if got[0].Title != "Searchable" {
		t.Errorf("result Title: got %q, want Searchable", got[0].Title)
	}
}

func TestSearchNoParams(t *testing.T) {
	all := []PageResult{
		{ID: "x", Title: "X"},
		{ID: "y", Title: "Y"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/search", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(all)
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	got, err := client.Search("proj", nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 results, got %d", len(got))
	}
}

func TestSearchByTag(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tag") != "auth" {
			t.Errorf("expected tag=auth, got %q", r.URL.Query().Get("tag"))
		}
		json.NewEncoder(w).Encode([]PageResult{{ID: "t-001", Title: "Auth Page"}})
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	got, err := client.Search("proj", map[string]string{"tag": "auth"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].ID != "t-001" {
		t.Errorf("unexpected results: %+v", got)
	}
}

// ---------------------------------------------------------------------------
// GetReferences
// ---------------------------------------------------------------------------

func TestGetReferences(t *testing.T) {
	// The server returns a RefNode root; client maps it to ReferencesResponse.
	root := RefNode{
		Ref:    "P-001",
		Status: "root",
		Children: []RefNode{
			{
				Ref:      "SRC-001",
				Status:   "valid",
				Version:  1,
				Checksum: "abc123",
				Children: []RefNode{},
			},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages/P-001/references", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("depth") != "2" {
			t.Errorf("expected depth=2, got %q", r.URL.Query().Get("depth"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(root)
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	got, err := client.GetReferences("proj", "P-001", 2)
	if err != nil {
		t.Fatalf("GetReferences: %v", err)
	}

	if got.PageID != "P-001" {
		t.Errorf("PageID: got %q, want P-001", got.PageID)
	}
	if len(got.Hard) != 1 {
		t.Fatalf("Hard len: got %d, want 1", len(got.Hard))
	}
	child := got.Hard[0]
	if child.Ref != "SRC-001" {
		t.Errorf("child Ref: got %q, want SRC-001", child.Ref)
	}
	if child.Status != "valid" {
		t.Errorf("child Status: got %q, want valid", child.Status)
	}
	if child.Version != 1 {
		t.Errorf("child Version: got %d, want 1", child.Version)
	}
	if child.Checksum != "abc123" {
		t.Errorf("child Checksum: got %q, want abc123", child.Checksum)
	}
}

func TestGetReferencesEmpty(t *testing.T) {
	root := RefNode{
		Ref:      "P-002",
		Status:   "root",
		Children: []RefNode{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages/P-002/references", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(root)
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	got, err := client.GetReferences("proj", "P-002", 1)
	if err != nil {
		t.Fatalf("GetReferences: %v", err)
	}
	if got.PageID != "P-002" {
		t.Errorf("PageID: got %q, want P-002", got.PageID)
	}
	if len(got.Hard) != 0 {
		t.Errorf("expected 0 children, got %d", len(got.Hard))
	}
}

func TestGetReferencesNestedChildren(t *testing.T) {
	// Verify nested tree is preserved through the client.
	root := RefNode{
		Ref:    "ROOT",
		Status: "root",
		Children: []RefNode{
			{
				Ref:    "CHILD-1",
				Status: "valid",
				Children: []RefNode{
					{Ref: "GRANDCHILD-1", Status: "valid", Children: []RefNode{}},
				},
			},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages/ROOT/references", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(root)
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	got, err := client.GetReferences("proj", "ROOT", 3)
	if err != nil {
		t.Fatalf("GetReferences: %v", err)
	}
	if len(got.Hard) != 1 {
		t.Fatalf("expected 1 child, got %d", len(got.Hard))
	}
	if len(got.Hard[0].Children) != 1 {
		t.Fatalf("expected 1 grandchild, got %d", len(got.Hard[0].Children))
	}
	if got.Hard[0].Children[0].Ref != "GRANDCHILD-1" {
		t.Errorf("grandchild Ref: got %q, want GRANDCHILD-1", got.Hard[0].Children[0].Ref)
	}
}

func TestGetReferencesProjectNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/nope/pages/P-001/references", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "project not found", http.StatusNotFound)
	})

	client, cleanup := newTestClient(t, mux)
	defer cleanup()

	_, err := client.GetReferences("nope", "P-001", 1)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

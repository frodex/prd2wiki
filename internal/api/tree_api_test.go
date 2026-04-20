package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestTreeUpdatePathEquivalence (T2.7) — same preserve-on-partial-PUT
// contract as TestUpdatePartialBodyPreservesFrontmatter, but via the Tree
// API (PUT /api/tree/{path}). Confirms tree_api.go:treeUpdatePage uses the
// same merge pattern as pages.go:upsertPage.
func TestTreeUpdatePathEquivalence(t *testing.T) {
	srv, token := setupAuthServer(t)
	handler := srv.Handler()

	// POST via Tree API to create a page with full frontmatter.
	postBody, _ := json.Marshal(TreeCreateRequest{
		Title:  "Tree Preserve Test",
		Type:   "research",
		Status: "superseded",
		Tags:   []string{"a", "b", "c"},
		Body:   "# t0",
		Author: "tree-creator@example.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tree/test-project/pages", bytes.NewReader(postBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("tree POST: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode POST resp: %v", err)
	}
	slug, _ := createResp["slug"].(string)
	if slug == "" {
		t.Fatalf("no slug in tree POST response: %v", createResp)
	}
	pageUUID, _ := createResp["id"].(string)
	if pageUUID == "" {
		t.Fatalf("no id in tree POST response: %v", createResp)
	}

	// Pre-check via git: full frontmatter landed on disk.
	repo := srv.repos["test-project"]
	gitPath := "pages/" + pageUUID + ".md"
	fmPre, _, err := repo.ReadPageWithMeta("draft/incoming", gitPath)
	if err != nil {
		t.Fatalf("ReadPageWithMeta pre-PUT: %v", err)
	}
	if fmPre.Type != "research" || fmPre.Status != "superseded" {
		t.Fatalf("tree POST fm: type=%q status=%q; want research/superseded", fmPre.Type, fmPre.Status)
	}
	if !tagsEqual(fmPre.Tags, []string{"a", "b", "c"}) {
		t.Fatalf("tree POST fm.Tags: %v; want [a b c]", fmPre.Tags)
	}

	// PUT via Tree API with partial body — only title and body, omit
	// type/status/tags/author.
	putBody := []byte(`{"title":"Tree Preserve Test","body":"# t1"}`)
	putReq := httptest.NewRequest(http.MethodPut, "/api/tree/test-project/"+slug, bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set("Authorization", "Bearer "+token)
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("tree PUT: expected 200, got %d: %s", putRec.Code, putRec.Body.String())
	}

	fmPost, _, err := repo.ReadPageWithMeta("draft/incoming", gitPath)
	if err != nil {
		t.Fatalf("ReadPageWithMeta post-PUT: %v", err)
	}
	if fmPost.ID != pageUUID {
		t.Errorf("fm.ID after tree PUT: got %q, want %q (tree entry UUID, defensive guard)", fmPost.ID, pageUUID)
	}
	if fmPost.Type != "research" {
		t.Errorf("fm.Type after tree partial-PUT: got %q, want research (preserved)", fmPost.Type)
	}
	if fmPost.Status != "superseded" {
		t.Errorf("fm.Status after tree partial-PUT: got %q, want superseded (preserved)", fmPost.Status)
	}
	if !tagsEqual(fmPost.Tags, []string{"a", "b", "c"}) {
		t.Errorf("fm.Tags after tree partial-PUT: got %v, want [a b c] (preserved)", fmPost.Tags)
	}
	if fmPost.DCCreator != "tree-creator@example.com" {
		t.Errorf("fm.DCCreator after tree partial-PUT: got %q, want tree-creator@example.com (preserved)", fmPost.DCCreator)
	}
	if fmPost.DCModified.Time.IsZero() {
		t.Errorf("fm.DCModified after tree PUT: zero; want populated on every write")
	}
}

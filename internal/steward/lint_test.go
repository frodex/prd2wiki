package steward

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/frodex/prd2wiki/internal/mcp"
)

// ---- helpers ----------------------------------------------------------------

// pageList encodes a slice of PageResult as JSON for the list-pages endpoint.
func pageList(pages []mcp.PageResult) []byte {
	b, _ := json.Marshal(pages)
	return b
}

// refNode encodes a RefNode tree as JSON for the references endpoint.
func refNode(node mcp.RefNode) []byte {
	b, _ := json.Marshal(node)
	return b
}

// ---- TestLintClean ----------------------------------------------------------

// TestLintClean verifies that a clean project with valid provenance produces
// no error or warning findings.
func TestLintClean(t *testing.T) {
	// Two pages: "parent" references "child".
	pages := []mcp.PageResult{
		{ID: "p-parent", Title: "Parent", Type: "page", Status: "published"},
		{ID: "p-child", Title: "Child", Type: "source", Status: "published"},
	}

	refs := map[string]mcp.RefNode{
		"p-parent": {
			Ref:    "p-parent",
			Status: "published",
			Children: []mcp.RefNode{
				{Ref: "p-child", Status: "published", Children: nil},
			},
		},
		"p-child": {
			Ref:      "p-child",
			Status:   "published",
			Children: nil,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pages") && !strings.Contains(r.URL.Path, "/references"):
			w.Header().Set("Content-Type", "application/json")
			w.Write(pageList(pages))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/references"):
			// Extract page ID from path: /api/projects/{project}/pages/{id}/references
			parts := strings.Split(r.URL.Path, "/")
			id := parts[len(parts)-2]
			w.Header().Set("Content-Type", "application/json")
			if node, ok := refs[id]; ok {
				w.Write(refNode(node))
			} else {
				// No references: return empty root node.
				w.Write(refNode(mcp.RefNode{Ref: id, Status: "published"}))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := mcp.NewWikiClient(srv.URL)
	steward := NewLintSteward(client)

	report, err := steward.Run("testproject")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	counts := report.CountBySeverity()
	if counts["error"] != 0 {
		t.Errorf("expected 0 errors, got %d: %v", counts["error"], report.Findings)
	}
	if counts["warning"] != 0 {
		t.Errorf("expected 0 warnings, got %d: %v", counts["warning"], report.Findings)
	}
	if report.Steward != "lint" {
		t.Errorf("expected steward 'lint', got %s", report.Steward)
	}
	if !strings.HasPrefix(report.Summary, "Lint complete:") {
		t.Errorf("unexpected summary format: %s", report.Summary)
	}
}

// ---- TestLintStaleProvenance ------------------------------------------------

// TestLintStaleProvenance verifies that a page with a stale reference produces
// an error-level finding.
func TestLintStaleProvenance(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "p-main", Title: "Main", Type: "page", Status: "published"},
		{ID: "p-stale-src", Title: "Stale Source", Type: "source", Status: "stale"},
	}

	refs := map[string]mcp.RefNode{
		"p-main": {
			Ref:    "p-main",
			Status: "published",
			Children: []mcp.RefNode{
				{Ref: "p-stale-src", Status: "stale", Children: nil},
			},
		},
		"p-stale-src": {
			Ref:      "p-stale-src",
			Status:   "stale",
			Children: nil,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pages"):
			w.Header().Set("Content-Type", "application/json")
			w.Write(pageList(pages))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/references"):
			parts := strings.Split(r.URL.Path, "/")
			id := parts[len(parts)-2]
			w.Header().Set("Content-Type", "application/json")
			if node, ok := refs[id]; ok {
				w.Write(refNode(node))
			} else {
				w.Write(refNode(mcp.RefNode{Ref: id}))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := mcp.NewWikiClient(srv.URL)
	steward := NewLintSteward(client)

	report, err := steward.Run("testproject")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	counts := report.CountBySeverity()
	if counts["error"] == 0 {
		t.Fatalf("expected at least 1 error finding for stale provenance, got 0")
	}

	// Verify the error finding mentions the stale reference.
	found := false
	for _, f := range report.Findings {
		if f.Severity == "error" && f.PageID == "p-main" && strings.Contains(f.Message, "stale") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error finding for p-main with stale provenance, findings: %v", report.Findings)
	}
}

// ---- TestLintContestedPage --------------------------------------------------

// TestLintContestedPage verifies that a contested page produces a warning finding.
func TestLintContestedPage(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "p-contested", Title: "Disputed Page", Type: "page", Status: "contested"},
		{ID: "p-normal", Title: "Normal Page", Type: "source", Status: "published"},
	}

	refs := map[string]mcp.RefNode{
		"p-contested": {
			Ref:    "p-contested",
			Status: "contested",
			Children: []mcp.RefNode{
				{Ref: "p-normal", Status: "published"},
			},
		},
		"p-normal": {
			Ref:      "p-normal",
			Status:   "published",
			Children: nil,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pages"):
			w.Header().Set("Content-Type", "application/json")
			w.Write(pageList(pages))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/references"):
			parts := strings.Split(r.URL.Path, "/")
			id := parts[len(parts)-2]
			w.Header().Set("Content-Type", "application/json")
			if node, ok := refs[id]; ok {
				w.Write(refNode(node))
			} else {
				w.Write(refNode(mcp.RefNode{Ref: id}))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := mcp.NewWikiClient(srv.URL)
	steward := NewLintSteward(client)

	report, err := steward.Run("testproject")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	counts := report.CountBySeverity()
	if counts["warning"] == 0 {
		t.Fatalf("expected at least 1 warning finding for contested page, got 0")
	}

	found := false
	for _, f := range report.Findings {
		if f.Severity == "warning" && f.PageID == "p-contested" && strings.Contains(f.Message, "contested") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning finding for contested page p-contested, findings: %v", report.Findings)
	}
}

// ---- TestLintOrphanedPage ---------------------------------------------------

// TestLintOrphanedPage verifies that a non-source page that is never referenced
// produces an info-level finding.
func TestLintOrphanedPage(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "p-index", Title: "Index", Type: "page", Status: "published"},
		{ID: "p-orphan", Title: "Lost Page", Type: "page", Status: "published"},
		{ID: "p-src", Title: "Source A", Type: "source", Status: "published"},
	}

	refs := map[string]mcp.RefNode{
		// p-index references p-src; p-orphan is never referenced.
		"p-index": {
			Ref:    "p-index",
			Status: "published",
			Children: []mcp.RefNode{
				{Ref: "p-src", Status: "published"},
			},
		},
		"p-orphan": {
			Ref:      "p-orphan",
			Status:   "published",
			Children: nil,
		},
		"p-src": {
			Ref:      "p-src",
			Status:   "published",
			Children: nil,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pages"):
			w.Header().Set("Content-Type", "application/json")
			w.Write(pageList(pages))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/references"):
			parts := strings.Split(r.URL.Path, "/")
			id := parts[len(parts)-2]
			w.Header().Set("Content-Type", "application/json")
			if node, ok := refs[id]; ok {
				w.Write(refNode(node))
			} else {
				w.Write(refNode(mcp.RefNode{Ref: id}))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := mcp.NewWikiClient(srv.URL)
	steward := NewLintSteward(client)

	report, err := steward.Run("testproject")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	counts := report.CountBySeverity()
	if counts["info"] == 0 {
		t.Fatalf("expected at least 1 info finding for orphaned page, got 0")
	}

	foundOrphan := false
	for _, f := range report.Findings {
		if f.Severity == "info" && f.PageID == "p-orphan" {
			foundOrphan = true
			break
		}
	}
	if !foundOrphan {
		t.Errorf("expected info finding for orphan p-orphan, findings: %v", report.Findings)
	}

	// p-index should NOT be flagged as an orphan even though it is not referenced
	// (it is the root/index page — and for this test p-index is also not referenced,
	// but the important thing is p-orphan is flagged; we verify p-src is NOT flagged
	// because it is type=source).
	for _, f := range report.Findings {
		if f.Severity == "info" && f.PageID == "p-src" {
			t.Errorf("source page p-src should not be flagged as an orphan")
		}
	}
}

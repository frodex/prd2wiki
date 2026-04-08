package steward

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/frodex/prd2wiki/internal/mcp"
)

// newResolveTestServer starts a mock HTTP server with the given mux and returns
// a ResolveSteward pointed at it, plus a cleanup function.
func newResolveTestServer(t *testing.T, mux *http.ServeMux) (*ResolveSteward, func()) {
	t.Helper()
	srv := httptest.NewServer(mux)
	client := mcp.NewWikiClient(srv.URL)
	return NewResolveSteward(client), srv.Close
}

// ---------------------------------------------------------------------------
// TestResolveNoIssues
// ---------------------------------------------------------------------------

func TestResolveNoIssues(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "req-001", Title: "Active Req", Status: "active"},
		{ID: "req-002", Title: "Approved Req", Status: "approved"},
		{ID: "req-003", Title: "Draft Req", Status: "draft"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pages)
	})

	steward, cleanup := newResolveTestServer(t, mux)
	defer cleanup()

	report, err := steward.Run("proj")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Steward != "resolve" {
		t.Errorf("Steward: got %q, want resolve", report.Steward)
	}
	if report.Project != "proj" {
		t.Errorf("Project: got %q, want proj", report.Project)
	}
	if len(report.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d: %+v", len(report.Findings), report.Findings)
	}
	if !strings.Contains(report.Summary, "0 contested") {
		t.Errorf("Summary should mention 0 contested: %q", report.Summary)
	}
	if !strings.Contains(report.Summary, "0 stale") {
		t.Errorf("Summary should mention 0 stale: %q", report.Summary)
	}
	if !strings.Contains(report.Summary, "0 deprecated") {
		t.Errorf("Summary should mention 0 deprecated: %q", report.Summary)
	}
}

// ---------------------------------------------------------------------------
// TestResolveContested
// ---------------------------------------------------------------------------

func TestResolveContested(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "req-001", Title: "Active Req", Status: "active"},
		{ID: "req-002", Title: "Contested Req", Status: "contested"},
	}

	pageDetail := mcp.PageResponse{
		ID:     "req-002",
		Title:  "Contested Req",
		Status: "contested",
		Body:   "# Contested content",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pages)
	})
	mux.HandleFunc("GET /api/projects/proj/pages/req-002", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("branch") != "truth" {
			t.Errorf("expected branch=truth, got %q", r.URL.Query().Get("branch"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pageDetail)
	})

	steward, cleanup := newResolveTestServer(t, mux)
	defer cleanup()

	report, err := steward.Run("proj")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(report.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(report.Findings), report.Findings)
	}

	f := report.Findings[0]
	if f.PageID != "req-002" {
		t.Errorf("finding PageID: got %q, want req-002", f.PageID)
	}
	if f.Severity != "warning" {
		t.Errorf("finding Severity: got %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Action, "challenge branch") {
		t.Errorf("action should mention 'challenge branch': %q", f.Action)
	}
	if !strings.Contains(f.Action, "[!decision]") {
		t.Errorf("action should mention '[!decision]': %q", f.Action)
	}

	if !strings.Contains(report.Summary, "1 contested") {
		t.Errorf("Summary should mention 1 contested: %q", report.Summary)
	}
}

// ---------------------------------------------------------------------------
// TestResolveContested with contested_by field
// ---------------------------------------------------------------------------

func TestResolveContestedWithContestedBy(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "req-003", Title: "Challenged Req", Status: "contested"},
	}

	// Page response with contested_by in provenance
	rawProvenance := map[string]interface{}{
		"contested_by": "challenge/req-003-v2",
	}
	pageDetail := mcp.PageResponse{
		ID:         "req-003",
		Title:      "Challenged Req",
		Status:     "contested",
		Provenance: rawProvenance,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pages)
	})
	mux.HandleFunc("GET /api/projects/proj/pages/req-003", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pageDetail)
	})

	steward, cleanup := newResolveTestServer(t, mux)
	defer cleanup()

	report, err := steward.Run("proj")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(report.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(report.Findings))
	}

	f := report.Findings[0]
	if !strings.Contains(f.Message, "challenge/req-003-v2") {
		t.Errorf("message should reference contested_by branch: %q", f.Message)
	}
}

// ---------------------------------------------------------------------------
// TestResolveStale
// ---------------------------------------------------------------------------

func TestResolveStale(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "req-001", Title: "Active Req", Status: "active"},
		{ID: "req-010", Title: "Stale Req", Status: "stale"},
	}

	refs := mcp.RefNode{
		Ref:    "req-010",
		Status: "root",
		Children: []mcp.RefNode{
			{Ref: "SRC-042", Status: "changed", Checksum: "old-sum"},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pages)
	})
	mux.HandleFunc("GET /api/projects/proj/pages/req-010/references", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(refs)
	})

	steward, cleanup := newResolveTestServer(t, mux)
	defer cleanup()

	report, err := steward.Run("proj")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(report.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(report.Findings), report.Findings)
	}

	f := report.Findings[0]
	if f.PageID != "req-010" {
		t.Errorf("finding PageID: got %q, want req-010", f.PageID)
	}
	if f.Severity != "warning" {
		t.Errorf("finding Severity: got %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Action, "SRC-042") {
		t.Errorf("action should mention source SRC-042: %q", f.Action)
	}
	if !strings.Contains(f.Action, "checksum") {
		t.Errorf("action should mention checksum: %q", f.Action)
	}

	if !strings.Contains(report.Summary, "1 stale") {
		t.Errorf("Summary should mention 1 stale: %q", report.Summary)
	}
}

// ---------------------------------------------------------------------------
// TestResolveStaleNoRefs
// ---------------------------------------------------------------------------

func TestResolveStaleNoRefs(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "req-020", Title: "Stale No Refs", Status: "stale"},
	}

	refs := mcp.RefNode{
		Ref:      "req-020",
		Status:   "root",
		Children: []mcp.RefNode{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pages)
	})
	mux.HandleFunc("GET /api/projects/proj/pages/req-020/references", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(refs)
	})

	steward, cleanup := newResolveTestServer(t, mux)
	defer cleanup()

	report, err := steward.Run("proj")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(report.Findings) != 1 {
		t.Fatalf("expected 1 finding for stale page with no refs, got %d", len(report.Findings))
	}

	f := report.Findings[0]
	if f.PageID != "req-020" {
		t.Errorf("finding PageID: got %q, want req-020", f.PageID)
	}
}

// ---------------------------------------------------------------------------
// TestResolveDeprecated
// ---------------------------------------------------------------------------

func TestResolveDeprecated(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "req-100", Title: "Old Req", Status: "deprecated"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pages)
	})

	steward, cleanup := newResolveTestServer(t, mux)
	defer cleanup()

	report, err := steward.Run("proj")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(report.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(report.Findings), report.Findings)
	}

	f := report.Findings[0]
	if f.PageID != "req-100" {
		t.Errorf("finding PageID: got %q, want req-100", f.PageID)
	}
	if f.Severity != "info" {
		t.Errorf("finding Severity: got %q, want info", f.Severity)
	}
	if !strings.Contains(f.Message, "deprecated") {
		t.Errorf("message should mention deprecated: %q", f.Message)
	}
	if !strings.Contains(f.Action, "superseded_by") {
		t.Errorf("action should mention superseded_by: %q", f.Action)
	}
	if !strings.Contains(f.Action, "archive") {
		t.Errorf("action should mention archive: %q", f.Action)
	}

	if !strings.Contains(report.Summary, "1 deprecated") {
		t.Errorf("Summary should mention 1 deprecated: %q", report.Summary)
	}
}

// ---------------------------------------------------------------------------
// TestResolveMixed
// ---------------------------------------------------------------------------

func TestResolveMixed(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "req-001", Title: "Active", Status: "active"},
		{ID: "req-002", Title: "Contested", Status: "contested"},
		{ID: "req-003", Title: "Stale", Status: "stale"},
		{ID: "req-004", Title: "Deprecated", Status: "deprecated"},
		{ID: "req-005", Title: "Another Contested", Status: "contested"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pages)
	})
	mux.HandleFunc("GET /api/projects/proj/pages/req-002", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(mcp.PageResponse{ID: "req-002", Status: "contested"})
	})
	mux.HandleFunc("GET /api/projects/proj/pages/req-005", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(mcp.PageResponse{ID: "req-005", Status: "contested"})
	})
	mux.HandleFunc("GET /api/projects/proj/pages/req-003/references", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(mcp.RefNode{Ref: "req-003", Status: "root", Children: []mcp.RefNode{}})
	})

	steward, cleanup := newResolveTestServer(t, mux)
	defer cleanup()

	report, err := steward.Run("proj")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// 2 contested + 1 stale + 1 deprecated = 4 findings
	if len(report.Findings) != 4 {
		t.Fatalf("expected 4 findings, got %d: %+v", len(report.Findings), report.Findings)
	}

	if !strings.Contains(report.Summary, "2 contested") {
		t.Errorf("Summary should mention 2 contested: %q", report.Summary)
	}
	if !strings.Contains(report.Summary, "1 stale") {
		t.Errorf("Summary should mention 1 stale: %q", report.Summary)
	}
	if !strings.Contains(report.Summary, "1 deprecated") {
		t.Errorf("Summary should mention 1 deprecated: %q", report.Summary)
	}
}

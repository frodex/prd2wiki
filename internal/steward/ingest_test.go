package steward

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/frodex/prd2wiki/internal/mcp"
)

// buildTestServer returns an httptest.Server that serves pages from the given
// list on GET /api/projects/{project}/pages and individual page detail on
// GET /api/projects/{project}/pages/{id}.
func buildTestServer(t *testing.T, pages []mcp.PageResult, details map[string]*mcp.PageResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Match GET /api/projects/{project}/pages/{id}
		// vs   GET /api/projects/{project}/pages
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/projects/"), "/")
		// parts[0] = project, parts[1] = "pages", parts[2] (optional) = id
		if len(parts) == 3 && parts[1] == "pages" {
			id := parts[2]
			if d, ok := details[id]; ok {
				_ = json.NewEncoder(w).Encode(d)
				return
			}
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}

		// List pages
		_ = json.NewEncoder(w).Encode(pages)
	}))
}

// ---------------------------------------------------------------------------
// TestIngestClean — all pages are active/L3, no source pages — no major findings
// ---------------------------------------------------------------------------

func TestIngestClean(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "P-001", Title: "Architecture Overview", Type: "policy", Status: "active", TrustLevel: 3},
		{ID: "P-002", Title: "Deployment Guide", Type: "guide", Status: "active", TrustLevel: 3},
		{ID: "P-003", Title: "API Reference", Type: "reference", Status: "active", TrustLevel: 2},
	}

	srv := buildTestServer(t, pages, nil)
	defer srv.Close()

	client := mcp.NewWikiClient(srv.URL)
	steward := NewIngestSteward(client)

	report, err := steward.Run("myproject")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if report.Steward != "ingest" {
		t.Errorf("expected steward='ingest', got %q", report.Steward)
	}
	if report.Project != "myproject" {
		t.Errorf("expected project='myproject', got %q", report.Project)
	}

	// No warnings or errors expected for a clean dataset
	counts := report.CountBySeverity()
	if counts["warning"] > 0 {
		t.Errorf("expected 0 warnings for clean data, got %d", counts["warning"])
	}
	if counts["error"] > 0 {
		t.Errorf("expected 0 errors for clean data, got %d", counts["error"])
	}

	if report.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

// ---------------------------------------------------------------------------
// TestIngestDraftPages — pages with status=draft trigger info findings
// ---------------------------------------------------------------------------

func TestIngestDraftPages(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "P-001", Title: "Architecture Overview", Type: "policy", Status: "active", TrustLevel: 3},
		{ID: "P-002", Title: "Draft Feature Spec", Type: "spec", Status: "draft", TrustLevel: 1},
		{ID: "P-003", Title: "Another Draft", Type: "guide", Status: "draft", TrustLevel: 0},
	}

	srv := buildTestServer(t, pages, nil)
	defer srv.Close()

	client := mcp.NewWikiClient(srv.URL)
	steward := NewIngestSteward(client)

	report, err := steward.Run("myproject")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	counts := report.CountBySeverity()
	// Expect at least 2 info findings (one per draft page)
	if counts["info"] < 2 {
		t.Errorf("expected at least 2 info findings for draft pages, got %d", counts["info"])
	}

	// Verify the info findings mention the draft pages
	found002, found003 := false, false
	for _, f := range report.Findings {
		if f.Severity == "info" && strings.Contains(f.Message, "P-002") {
			found002 = true
		}
		if f.Severity == "info" && strings.Contains(f.Message, "P-003") {
			found003 = true
		}
	}
	if !found002 {
		t.Error("expected info finding referencing P-002 (draft page)")
	}
	if !found003 {
		t.Error("expected info finding referencing P-003 (draft/L0 page)")
	}

	// Summary should mention pages needing review
	if !strings.Contains(report.Summary, "review") {
		t.Errorf("expected summary to mention 'review', got: %s", report.Summary)
	}
}

// ---------------------------------------------------------------------------
// TestIngestTrustLevelZero — pages with trust_level=0 also trigger info findings
// ---------------------------------------------------------------------------

func TestIngestTrustLevelZero(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "P-001", Title: "New Import", Type: "guide", Status: "active", TrustLevel: 0},
		{ID: "P-002", Title: "Stable Page", Type: "guide", Status: "active", TrustLevel: 2},
	}

	srv := buildTestServer(t, pages, nil)
	defer srv.Close()

	client := mcp.NewWikiClient(srv.URL)
	steward := NewIngestSteward(client)

	report, err := steward.Run("myproject")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	counts := report.CountBySeverity()
	if counts["info"] < 1 {
		t.Errorf("expected at least 1 info finding for trust_level=0 page, got %d", counts["info"])
	}

	var foundP001 bool
	for _, f := range report.Findings {
		if f.Severity == "info" && strings.Contains(f.Message, "P-001") {
			foundP001 = true
		}
	}
	if !foundP001 {
		t.Error("expected info finding referencing P-001 (L0 page)")
	}
}

// ---------------------------------------------------------------------------
// TestIngestDuplicateTitles — similar titles produce a warning
// ---------------------------------------------------------------------------

func TestIngestDuplicateTitles(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "P-001", Title: "Deployment Guide", Type: "guide", Status: "active", TrustLevel: 3},
		{ID: "P-002", Title: "deployment guide", Type: "guide", Status: "active", TrustLevel: 3},
		{ID: "P-003", Title: "API Reference", Type: "reference", Status: "active", TrustLevel: 3},
	}

	srv := buildTestServer(t, pages, nil)
	defer srv.Close()

	client := mcp.NewWikiClient(srv.URL)
	steward := NewIngestSteward(client)

	report, err := steward.Run("myproject")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	counts := report.CountBySeverity()
	if counts["warning"] < 1 {
		t.Errorf("expected at least 1 warning for duplicate titles, got %d", counts["warning"])
	}

	// The warning should mention both pages
	var foundDupWarning bool
	for _, f := range report.Findings {
		if f.Severity == "warning" &&
			(strings.Contains(f.Message, "P-001") || strings.Contains(f.Message, "P-002")) {
			foundDupWarning = true
			break
		}
	}
	if !foundDupWarning {
		t.Error("expected warning finding referencing P-001 and/or P-002 (duplicate titles)")
	}

	// Summary should mention potential duplicates
	if !strings.Contains(report.Summary, "duplicate") {
		t.Errorf("expected summary to mention 'duplicate', got: %s", report.Summary)
	}
}

// ---------------------------------------------------------------------------
// TestIngestSubstringTitles — one title is a substring of another
// ---------------------------------------------------------------------------

func TestIngestSubstringTitles(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "P-001", Title: "API Reference", Type: "reference", Status: "active", TrustLevel: 3},
		{ID: "P-002", Title: "API Reference Guide", Type: "reference", Status: "active", TrustLevel: 3},
	}

	srv := buildTestServer(t, pages, nil)
	defer srv.Close()

	client := mcp.NewWikiClient(srv.URL)
	steward := NewIngestSteward(client)

	report, err := steward.Run("myproject")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	counts := report.CountBySeverity()
	if counts["warning"] < 1 {
		t.Errorf("expected at least 1 warning for substring title match, got %d", counts["warning"])
	}
}

// ---------------------------------------------------------------------------
// TestIngestIncompleteSourcePage — source page with short body triggers warning
// ---------------------------------------------------------------------------

func TestIngestIncompleteSourcePage(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "SRC-001", Title: "Source Doc", Type: "source", Status: "active", TrustLevel: 2},
		{ID: "P-001", Title: "Full Page", Type: "policy", Status: "active", TrustLevel: 3},
	}

	details := map[string]*mcp.PageResponse{
		"SRC-001": {
			ID:         "SRC-001",
			Title:      "Source Doc",
			Type:       "source",
			Status:     "active",
			TrustLevel: 2,
			Body:       "Short.", // < 50 chars
		},
	}

	srv := buildTestServer(t, pages, details)
	defer srv.Close()

	client := mcp.NewWikiClient(srv.URL)
	steward := NewIngestSteward(client)

	report, err := steward.Run("myproject")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	counts := report.CountBySeverity()
	if counts["warning"] < 1 {
		t.Errorf("expected at least 1 warning for incomplete source page, got %d", counts["warning"])
	}

	var foundSrc bool
	for _, f := range report.Findings {
		if f.Severity == "warning" && strings.Contains(f.Message, "SRC-001") {
			foundSrc = true
		}
	}
	if !foundSrc {
		t.Error("expected warning finding referencing SRC-001 (incomplete source page)")
	}

	// Summary should mention incomplete sources
	if !strings.Contains(report.Summary, "incomplete") {
		t.Errorf("expected summary to mention 'incomplete', got: %s", report.Summary)
	}
}

// ---------------------------------------------------------------------------
// TestIngestSummaryFormat — verify summary contains all three counters
// ---------------------------------------------------------------------------

func TestIngestSummaryFormat(t *testing.T) {
	pages := []mcp.PageResult{
		{ID: "P-001", Title: "Draft Page", Type: "guide", Status: "draft", TrustLevel: 1},
		{ID: "P-002", Title: "Active Page", Type: "guide", Status: "active", TrustLevel: 3},
	}

	srv := buildTestServer(t, pages, nil)
	defer srv.Close()

	client := mcp.NewWikiClient(srv.URL)
	steward := NewIngestSteward(client)

	report, err := steward.Run("myproject")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// Summary must follow the specified pattern
	if !strings.HasPrefix(report.Summary, "Ingest check:") {
		t.Errorf("expected summary to start with 'Ingest check:', got: %s", report.Summary)
	}
	if !strings.Contains(report.Summary, "review") {
		t.Errorf("expected summary to contain 'review', got: %s", report.Summary)
	}
	if !strings.Contains(report.Summary, "duplicate") {
		t.Errorf("expected summary to contain 'duplicate', got: %s", report.Summary)
	}
	if !strings.Contains(report.Summary, "incomplete") {
		t.Errorf("expected summary to contain 'incomplete', got: %s", report.Summary)
	}
}

package steward

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestReportJSON(t *testing.T) {
	r := &Report{
		Steward:   "lint",
		Project:   "test",
		Timestamp: time.Now(),
		Findings: []Finding{
			{Severity: "error", PageID: "P-001", Message: "broken provenance"},
			{Severity: "warning", PageID: "P-002", Message: "orphaned page"},
		},
	}
	data, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() returned error: %v", err)
	}

	// Verify it's valid JSON by round-tripping through unmarshal
	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON output is not valid JSON: %v", err)
	}

	// Verify both findings are present
	if len(decoded.Findings) != 2 {
		t.Errorf("expected 2 findings, got %d", len(decoded.Findings))
	}
	if decoded.Findings[0].PageID != "P-001" {
		t.Errorf("expected first finding page_id P-001, got %s", decoded.Findings[0].PageID)
	}
	if decoded.Findings[1].PageID != "P-002" {
		t.Errorf("expected second finding page_id P-002, got %s", decoded.Findings[1].PageID)
	}
	if decoded.Steward != "lint" {
		t.Errorf("expected steward 'lint', got %s", decoded.Steward)
	}
}

func TestCountBySeverity(t *testing.T) {
	r := &Report{
		Findings: []Finding{
			{Severity: "error", PageID: "P-001", Message: "first error"},
			{Severity: "error", PageID: "P-002", Message: "second error"},
			{Severity: "warning", PageID: "P-003", Message: "a warning"},
		},
	}
	counts := r.CountBySeverity()

	if counts["error"] != 2 {
		t.Errorf("expected 2 errors, got %d", counts["error"])
	}
	if counts["warning"] != 1 {
		t.Errorf("expected 1 warning, got %d", counts["warning"])
	}
	if counts["info"] != 0 {
		t.Errorf("expected 0 info, got %d", counts["info"])
	}
}

func TestHasErrors(t *testing.T) {
	withErrors := &Report{
		Findings: []Finding{
			{Severity: "warning", PageID: "P-001", Message: "just a warning"},
			{Severity: "error", PageID: "P-002", Message: "an error"},
		},
	}
	if !withErrors.HasErrors() {
		t.Error("expected HasErrors() to return true when errors are present")
	}

	withoutErrors := &Report{
		Findings: []Finding{
			{Severity: "warning", PageID: "P-001", Message: "just a warning"},
			{Severity: "info", PageID: "P-002", Message: "an info"},
		},
	}
	if withoutErrors.HasErrors() {
		t.Error("expected HasErrors() to return false when no errors are present")
	}

	empty := &Report{}
	if empty.HasErrors() {
		t.Error("expected HasErrors() to return false for empty findings")
	}
}

func TestStewardPromptPreamble(t *testing.T) {
	preamble := StewardPromptPreamble()

	if !strings.Contains(preamble, "Verify before claiming") {
		t.Error("preamble missing 'Verify before claiming'")
	}
	if !strings.Contains(preamble, "Confident Architect") {
		t.Error("preamble missing 'Confident Architect'")
	}
	if !strings.Contains(preamble, "## Steward Behavioral Rules") {
		t.Error("preamble missing '## Steward Behavioral Rules' header")
	}
	if !strings.Contains(preamble, "## Anti-Patterns to Avoid") {
		t.Error("preamble missing '## Anti-Patterns to Avoid' header")
	}
	if !strings.Contains(preamble, "Premature Builder") {
		t.Error("preamble missing 'Premature Builder'")
	}
	if !strings.Contains(preamble, "Clean vs Complete") {
		t.Error("preamble missing 'Clean vs Complete'")
	}
	if !strings.Contains(preamble, "Constraints first") {
		t.Error("preamble missing 'Constraints first'")
	}
}

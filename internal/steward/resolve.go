package steward

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/frodex/prd2wiki/internal/mcp"
)

// ResolveSteward checks for contested, stale, and deprecated pages and reports
// what needs human attention. It does NOT auto-fix anything.
type ResolveSteward struct {
	client *mcp.WikiClient
}

// NewResolveSteward creates a ResolveSteward backed by the given WikiClient.
func NewResolveSteward(client *mcp.WikiClient) *ResolveSteward {
	return &ResolveSteward{client: client}
}

// Run performs a resolution check for the given project.
// It finds contested, stale, and deprecated pages and returns a Report with
// recommended actions for each. No mutations are made to the wiki.
func (r *ResolveSteward) Run(project string) (*Report, error) {
	report := &Report{
		Steward:   "resolve",
		Project:   project,
		Timestamp: time.Now(),
		Findings:  []Finding{},
	}

	pages, err := r.client.ListPages(project, nil)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}

	var contested, stale, deprecated int

	for _, p := range pages {
		switch p.Status {
		case "contested":
			contested++
			f, err := r.handleContested(project, p)
			if err != nil {
				return nil, err
			}
			report.Findings = append(report.Findings, f)

		case "stale":
			stale++
			findings, err := r.handleStale(project, p)
			if err != nil {
				return nil, err
			}
			report.Findings = append(report.Findings, findings...)

		case "deprecated":
			deprecated++
			report.Findings = append(report.Findings, r.handleDeprecated(p))
		}
	}

	report.Summary = fmt.Sprintf(
		"Resolution check: %d contested, %d stale, %d deprecated pages need attention",
		contested, stale, deprecated,
	)

	return report, nil
}

// handleContested fetches the page detail on the truth branch and produces a
// warning finding that directs a human to review and resolve the challenge.
func (r *ResolveSteward) handleContested(project string, p mcp.PageResult) (Finding, error) {
	detail, err := r.client.GetPage(project, p.ID, "truth")
	if err != nil {
		return Finding{}, fmt.Errorf("get contested page %s: %w", p.ID, err)
	}

	// Attempt to extract contested_by from provenance if it's a map.
	contestedBy := contestedByFromProvenance(detail.Provenance)

	var msg string
	if contestedBy != "" {
		msg = fmt.Sprintf("Page %q is contested by challenge branch %q", detail.Title, contestedBy)
	} else {
		msg = fmt.Sprintf("Page %q (id: %s) is contested", detail.Title, detail.ID)
	}

	return Finding{
		Severity: "warning",
		PageID:   p.ID,
		Message:  msg,
		Action:   "Review challenge branch and resolve — accept or reject with [!decision] block",
	}, nil
}

// handleStale fetches the page's reference tree to identify which sources
// changed and returns a finding per stale page.
func (r *ResolveSteward) handleStale(project string, p mcp.PageResult) ([]Finding, error) {
	refs, err := r.client.GetReferences(project, p.ID, 1)
	if err != nil {
		return nil, fmt.Errorf("get references for stale page %s: %w", p.ID, err)
	}

	// Collect changed source refs.
	sources := changedSources(refs.Hard)

	var action string
	if len(sources) > 0 {
		action = fmt.Sprintf(
			"Re-verify source %s and update checksum, or update page content to reflect new source version",
			strings.Join(sources, ", "),
		)
	} else {
		action = "Re-verify sources and update checksum, or update page content to reflect new source version"
	}

	return []Finding{
		{
			Severity: "warning",
			PageID:   p.ID,
			Message:  fmt.Sprintf("Page %q (id: %s) is stale — one or more sources may have changed", p.Title, p.ID),
			Action:   action,
		},
	}, nil
}

// handleDeprecated returns an info finding for a deprecated page.
func (r *ResolveSteward) handleDeprecated(p mcp.PageResult) Finding {
	return Finding{
		Severity: "info",
		PageID:   p.ID,
		Message:  fmt.Sprintf("Page %q (id: %s) is deprecated", p.Title, p.ID),
		Action:   "Page is deprecated — check if superseded_by is set, archive if no longer needed",
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// contestedByFromProvenance tries to extract "contested_by" from a provenance
// value that may be a map[string]interface{} or serialised JSON.
func contestedByFromProvenance(prov interface{}) string {
	if prov == nil {
		return ""
	}

	// Direct map (e.g. from test fixtures or already-decoded JSON objects).
	if m, ok := prov.(map[string]interface{}); ok {
		if v, ok := m["contested_by"]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}

	// Re-marshal and unmarshal to handle arbitrary concrete types returned by
	// json.Unmarshal when the target field is interface{}.
	data, err := json.Marshal(prov)
	if err != nil {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	if v, ok := m["contested_by"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// changedSources returns the Ref values from the top-level children.
// We return all refs because any of them could be the cause of staleness;
// the human must verify. A future version could filter by ref status.
func changedSources(nodes []mcp.RefNode) []string {
	if len(nodes) == 0 {
		return nil
	}
	refs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		refs = append(refs, n.Ref)
	}
	return refs
}

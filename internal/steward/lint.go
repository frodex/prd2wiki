package steward

import (
	"fmt"
	"log"
	"time"

	"github.com/frodex/prd2wiki/internal/mcp"
)

// LintSteward checks a project's wiki pages for provenance, conformance, and
// structural issues, then returns a Report summarising findings.
type LintSteward struct {
	client *mcp.WikiClient
}

// NewLintSteward creates a LintSteward that queries via the given WikiClient.
func NewLintSteward(client *mcp.WikiClient) *LintSteward {
	return &LintSteward{client: client}
}

// Run performs all lint checks against the given project and returns a Report.
func (l *LintSteward) Run(project string) (*Report, error) {
	report := &Report{
		Steward:   "lint",
		Project:   project,
		Timestamp: time.Now(),
	}

	// 1. List all pages.
	pages, err := l.client.ListPages(project, nil)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}

	// Build a set of page IDs for orphan detection.
	pageIDs := make(map[string]bool, len(pages))
	for _, p := range pages {
		pageIDs[p.ID] = true
	}

	// referenced tracks which page IDs appear as reference targets across all
	// provenance chains.  A page that is never a target is an orphan candidate.
	referenced := make(map[string]bool)

	// deprecatedTargets maps a deprecated page ID to true so we can flag pages
	// that cite deprecated sources.
	deprecatedTargets := make(map[string]bool)

	// 2. Provenance chain validation – and collect reference targets.
	for _, p := range pages {
		refs, err := l.client.GetReferences(project, p.ID, 1)
		if err != nil {
			log.Printf("steward/lint: warning: could not fetch references for page %s: %v", p.ID, err)
			continue
		}

		for _, ref := range refs.Hard {
			// Track target as referenced.
			referenced[ref.Ref] = true

			// Check for stale or contested references.
			if ref.Status == "stale" || ref.Status == "contested" {
				report.Findings = append(report.Findings, Finding{
					Severity: "error",
					PageID:   p.ID,
					Field:    "provenance",
					Message:  fmt.Sprintf("reference %s has status %q — provenance chain invalid", ref.Ref, ref.Status),
					Action:   "resolve or replace the stale/contested reference",
				})
			}

			// Track deprecated targets so we can flag dependents below.
			if ref.Status == "deprecated" {
				deprecatedTargets[ref.Ref] = true
			}
		}
	}

	// 3. Orphaned pages — info-level finding for pages never referenced.
	for _, p := range pages {
		// Source pages are expected to be cited, not cite others, so they are
		// excluded from the orphan check.
		if p.Type == "source" {
			continue
		}
		if !referenced[p.ID] {
			report.Findings = append(report.Findings, Finding{
				Severity: "info",
				PageID:   p.ID,
				Message:  fmt.Sprintf("page %q (%s) is not referenced by any other page", p.Title, p.ID),
				Action:   "link this page from a parent or index page, or delete it if it is no longer needed",
			})
		}
	}

	// 4. Non-conforming pages — pages with conformance=pending.
	for _, p := range pages {
		if p.Status == "pending" {
			report.Findings = append(report.Findings, Finding{
				Severity: "warning",
				PageID:   p.ID,
				Message:  fmt.Sprintf("page %q has conformance status %q", p.Title, p.Status),
				Action:   "complete conformance review for this page",
			})
		}
	}

	// 5. Contested pages.
	for _, p := range pages {
		if p.Status == "contested" {
			report.Findings = append(report.Findings, Finding{
				Severity: "warning",
				PageID:   p.ID,
				Message:  fmt.Sprintf("page %q is contested and needs resolution", p.Title),
				Action:   "resolve the dispute and update the page status",
			})
		}
	}

	// 6. Deprecated source dependents — pages that cite a deprecated target.
	for _, p := range pages {
		refs, err := l.client.GetReferences(project, p.ID, 1)
		if err != nil {
			// Already warned above; skip silently here.
			continue
		}
		for _, ref := range refs.Hard {
			if deprecatedTargets[ref.Ref] {
				report.Findings = append(report.Findings, Finding{
					Severity: "error",
					PageID:   p.ID,
					Field:    "provenance",
					Message:  fmt.Sprintf("page cites deprecated source %s", ref.Ref),
					Action:   "replace the deprecated reference with a current source",
				})
			}
		}
	}

	// 7. Generate summary.
	counts := report.CountBySeverity()
	report.Summary = fmt.Sprintf(
		"Lint complete: %d errors, %d warnings, %d info across %d pages",
		counts["error"], counts["warning"], counts["info"], len(pages),
	)

	return report, nil
}

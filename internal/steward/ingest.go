package steward

import (
	"fmt"
	"strings"
	"time"

	"github.com/frodex/prd2wiki/internal/mcp"
)

// IngestSteward checks pages for issues that commonly arise after bulk ingestion.
type IngestSteward struct {
	client *mcp.WikiClient
}

// NewIngestSteward constructs an IngestSteward that talks to the given WikiClient.
func NewIngestSteward(client *mcp.WikiClient) *IngestSteward {
	return &IngestSteward{client: client}
}

// Run performs all ingest checks for the given project and returns a Report.
//
// Checks performed:
//  1. Non-conforming pages (status=draft or trust_level=0) — info finding.
//  2. Source pages with incomplete metadata (body < 50 chars or generic title) — warning.
//  3. Duplicate/similar title detection — warning.
//
// The report summary follows the pattern:
//
//	"Ingest check: X pages need review, Y potential duplicates, Z incomplete sources"
func (s *IngestSteward) Run(project string) (*Report, error) {
	pages, err := s.client.ListPages(project, nil)
	if err != nil {
		return nil, fmt.Errorf("ingest steward: list pages: %w", err)
	}

	report := &Report{
		Steward:   "ingest",
		Project:   project,
		Timestamp: time.Now(),
	}

	needsReview := 0
	potentialDuplicates := 0
	incompleteSourceCount := 0

	// --- Step 1: non-conforming pages (draft or trust_level=0) ---
	for _, p := range pages {
		if p.Status == "draft" || p.TrustLevel == 0 {
			label := "draft"
			if p.Status != "draft" {
				label = "L0"
			}
			report.Findings = append(report.Findings, Finding{
				Severity: "info",
				PageID:   p.ID,
				Message:  fmt.Sprintf("Page %s is %s — needs review and promotion", p.ID, label),
				Action:   "review and promote",
			})
			needsReview++
		}
	}

	// --- Step 2: source pages without complete metadata ---
	for _, p := range pages {
		if p.Type != "source" {
			continue
		}
		detail, err := s.client.GetPage(project, p.ID, "")
		if err != nil {
			// Skip pages we cannot retrieve — don't abort the whole run.
			continue
		}
		shortBody := len(strings.TrimSpace(detail.Body)) < 50
		genericTitle := isGenericTitle(detail.Title)
		if shortBody || genericTitle {
			report.Findings = append(report.Findings, Finding{
				Severity: "warning",
				PageID:   p.ID,
				Message:  fmt.Sprintf("Source page %s may need richer description", p.ID),
				Action:   "enrich source metadata",
			})
			incompleteSourceCount++
		}
	}

	// --- Step 3: duplicate title detection ---
	type titleEntry struct {
		id    string
		lower string
	}
	entries := make([]titleEntry, 0, len(pages))
	for _, p := range pages {
		entries = append(entries, titleEntry{id: p.ID, lower: strings.ToLower(p.Title)})
	}

	reported := make(map[string]bool)
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			a, b := entries[i], entries[j]
			pairKey := a.id + "|" + b.id
			if reported[pairKey] {
				continue
			}
			if isSimilarTitle(a.lower, b.lower) {
				report.Findings = append(report.Findings, Finding{
					Severity: "warning",
					PageID:   a.id,
					Message: fmt.Sprintf(
						"Pages %s and %s have similar titles — possible duplicate",
						a.id, b.id,
					),
					Action: "review for duplication",
				})
				reported[pairKey] = true
				potentialDuplicates++
			}
		}
	}

	// --- Summary ---
	report.Summary = fmt.Sprintf(
		"Ingest check: %d pages need review, %d potential duplicates, %d incomplete sources",
		needsReview, potentialDuplicates, incompleteSourceCount,
	)

	return report, nil
}

// isGenericTitle returns true for titles that look like placeholder or untitled content.
func isGenericTitle(title string) bool {
	generic := []string{"untitled", "new page", "page", "draft", "source", "document", "doc"}
	lower := strings.ToLower(strings.TrimSpace(title))
	for _, g := range generic {
		if lower == g {
			return true
		}
	}
	return false
}

// isSimilarTitle returns true when two lower-cased titles are considered suspiciously similar:
// exact match (case-insensitive) or one is a substring of the other.
func isSimilarTitle(a, b string) bool {
	if a == b {
		return true
	}
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return true
	}
	return false
}

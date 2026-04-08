// backfill imports local docs into the wiki git repo with original timestamps.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/schema"
)

type entry struct {
	id     string
	file   string
	date   string // RFC3339
	author string
	title  string
	typ    string
	tags   []string
	msg    string
}

func main() {
	dataDir := "./data"
	project := "default"
	branch := "draft/incoming"

	repo, err := wgit.OpenRepo(dataDir, project)
	if err != nil {
		log.Fatalf("open repo: %v", err)
	}

	entries := []entry{
		// Research journal
		{id: "JOURNAL-concept", file: "docs/research/2026-04-07-v0.1-prd-wiki-concept-journal.md",
			date: "2026-04-07T14:46:53-05:00", author: "claude", title: "PRD Wiki Concept Journal",
			typ: "concept", tags: []string{"research", "journal", "brainstorming"},
			msg: "research: PRD wiki concept journal v0.1"},

		// Spec v0.1
		{id: "SPEC-prd2wiki-design", file: "docs/superpowers/specs/2026-04-07-prd2wiki-design.md",
			date: "2026-04-07T14:46:53-05:00", author: "claude", title: "prd2wiki Design Specification",
			typ: "requirement", tags: []string{"spec", "design", "architecture"},
			msg: "spec: prd2wiki design v0.1 — initial draft"},

		// Spec v0.2
		{id: "SPEC-prd2wiki-design", file: "docs/superpowers/specs/2026-04-07-prd2wiki-design-02.md",
			date: "2026-04-07T15:42:38-05:00", author: "claude", title: "prd2wiki Design Specification",
			typ: "requirement", tags: []string{"spec", "design", "architecture"},
			msg: "spec: prd2wiki design v0.2 — expanded acronyms, work surface as primary feature"},

		// Spec v0.3
		{id: "SPEC-prd2wiki-design", file: "docs/superpowers/specs/2026-04-07-prd2wiki-design-03.md",
			date: "2026-04-07T15:58:43-05:00", author: "claude", title: "prd2wiki Design Specification",
			typ: "requirement", tags: []string{"spec", "design", "architecture"},
			msg: "spec: prd2wiki design v0.3 — added reference trees"},

		// Spec v0.4
		{id: "SPEC-prd2wiki-design", file: "docs/superpowers/specs/2026-04-07-prd2wiki-design-04.md",
			date: "2026-04-07T17:32:08-05:00", author: "claude", title: "prd2wiki Design Specification",
			typ: "requirement", tags: []string{"spec", "design", "architecture"},
			msg: "spec: prd2wiki design v0.4 — work surface elevated, sidecar API, agent remediation"},

		// Greg's NOTES on v0.1 (mutation of same page)
		{id: "SPEC-prd2wiki-design", file: "docs/superpowers/specs/2026-04-07-prd2wiki-design-NOTES.md",
			date: "2026-04-08T00:35:24-05:00", author: "greg", title: "prd2wiki Design Specification",
			typ: "requirement", tags: []string{"spec", "design", "architecture", "user-notes"},
			msg: "notes: greg's review of design spec — expand acronyms, work surface is primary"},

		// Greg's NOTES on v0.3 (mutation of same page)
		{id: "SPEC-prd2wiki-design", file: "docs/superpowers/specs/2026-04-07-prd2wiki-design-03-NOTES.md",
			date: "2026-04-08T00:35:55-05:00", author: "greg", title: "prd2wiki Design Specification",
			typ: "requirement", tags: []string{"spec", "design", "architecture", "user-notes"},
			msg: "notes: greg's review of v0.3 — agent remediation, self-deprecation on newer docs"},

		// Bibliography
		{id: "REF-bibliography", file: "docs/bibliography.md",
			date: "2026-04-07T19:25:35-05:00", author: "claude", title: "Bibliography — prd2wiki",
			typ: "reference", tags: []string{"bibliography", "sources"},
			msg: "reference: project bibliography"},

		// Phase 1-5 plans
		{id: "PLAN-phase1-wiki-core", file: "docs/superpowers/plans/2026-04-07-phase1-wiki-core.md",
			date: "2026-04-07T19:33:08-05:00", author: "claude", title: "Phase 1: Wiki Core — Implementation Plan",
			typ: "task", tags: []string{"plan", "implementation", "phase-1"},
			msg: "plan: Phase 1 — wiki core implementation"},

		{id: "PLAN-phase2-librarian", file: "docs/superpowers/plans/2026-04-07-phase2-librarian-vectordb.md",
			date: "2026-04-07T20:31:19-05:00", author: "claude", title: "Phase 2: Librarian + Vector Index",
			typ: "task", tags: []string{"plan", "implementation", "phase-2"},
			msg: "plan: Phase 2 — librarian + vector index"},

		{id: "PLAN-phase3-web-ui", file: "docs/superpowers/plans/2026-04-07-phase3-web-ui.md",
			date: "2026-04-07T20:47:28-05:00", author: "claude", title: "Phase 3: Web UI",
			typ: "task", tags: []string{"plan", "implementation", "phase-3"},
			msg: "plan: Phase 3 — web UI"},

		{id: "PLAN-phase4-mcp-sidecar", file: "docs/superpowers/plans/2026-04-07-phase4-mcp-sidecar.md",
			date: "2026-04-07T21:00:26-05:00", author: "claude", title: "Phase 4: MCP Sidecar",
			typ: "task", tags: []string{"plan", "implementation", "phase-4"},
			msg: "plan: Phase 4 — MCP sidecar"},

		{id: "PLAN-phase5-steward-agents", file: "docs/superpowers/plans/2026-04-07-phase5-steward-agents.md",
			date: "2026-04-07T21:11:17-05:00", author: "claude", title: "Phase 5: Steward Agents",
			typ: "task", tags: []string{"plan", "implementation", "phase-5"},
			msg: "plan: Phase 5 — steward agents"},

		// UX bugs journal
		{id: "JOURNAL-ux-bugs", file: "docs/research/2026-04-08-v0.1-ux-bugs-journal.md",
			date: "2026-04-08T00:08:14-05:00", author: "claude", title: "UX Bug Discovery Journal v0.1",
			typ: "concept", tags: []string{"journal", "bugs", "testing", "ux"},
			msg: "journal: UX bugs discovered during first user testing"},
	}

	for i, e := range entries {
		body, err := os.ReadFile(filepath.Join(".", e.file))
		if err != nil {
			log.Printf("SKIP %s: %v", e.file, err)
			continue
		}

		t, err := time.Parse(time.RFC3339, e.date)
		if err != nil {
			log.Printf("SKIP %s: bad date: %v", e.id, err)
			continue
		}

		fm := &schema.Frontmatter{
			ID:        e.id,
			Title:     e.title,
			Type:      e.typ,
			Status:    "draft",
			Tags:      e.tags,
			DCCreator: e.author + "@prd2wiki",
			DCCreated: schema.Date{Time: t},
		}

		data, err := schema.Serialize(fm, body)
		if err != nil {
			log.Printf("SKIP %s: serialize: %v", e.id, err)
			continue
		}

		path := fmt.Sprintf("pages/%s.md", e.id)

		// Write directly with the backdated timestamp
		err = repo.WritePageWithDate(branch, path, data, e.msg, e.author+"@prd2wiki", t)
		if err != nil {
			log.Printf("FAIL %s: %v", e.id, err)
			continue
		}

		fmt.Printf("[%d/%d] %s ← %s (%s, %s)\n", i+1, len(entries), e.id, filepath.Base(e.file), e.author, e.date[:16])
	}

	fmt.Printf("\nDone. %d entries imported.\n", len(entries))
}

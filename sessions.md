# sessions.md — Live Project Context
# Backed up as sessions.md.{SESSION}-{STEP} before each optimize

---

## Project Identity

**Repo:** prd2wiki (`/srv/prd2wiki`)
**Branch:** `impl/vector-store-removal-phase2`
**What this is:** A wiki system for living documents — PRDs, specs, plans, reviews, feedback. Filesystem source of truth (markdown + YAML frontmatter), git history, SQLite FTS, Pippi librarian for vector search. Web UI + REST API + MCP tools for human and agent access.

---

## Active Direction

Documentation teardown and governance. As-built PRD fully restructured with 14 numbered sections, project health hub, governance section. Proposed document taxonomy (10 types, 7 statuses, artifact state model) and steward orchestrator spec awaiting review. Next: implement taxonomy in code, complete verification stamps on all pages, design where session records live.

---

## Operational Conventions

[2026-04-07] Design must use familiar, trusted building blocks. People should look at this project and see something recognizable built from standards they already trust (git, markdown, PRs, RFCs). Only invent where nothing established exists.
[2026-04-07] Architecture must be composable and recursive. New features/uses stem from a robust set of building blocks adapted via configuration, not rewrites. Everything that can be a wiki page should be a wiki page. Like PHAT-TOAD Rule 1: one implementation, adapters absorb differences.

---

## Key Technical Decisions

[2026-04-13] Document types: research, spec, plan, report, review, reference, feedback, tracking, project, _test (+ untyped default). Replaces old 8-type system.
[2026-04-13] Statuses: sketch, draft, proposed, approved, rejected, superseded, deprecated. Replaces old 9-status system.
[2026-04-13] Artifact states: not-started → in-progress → reviewable. Certifications: tested, verified, deployed, blocked (additive, combinable).
[2026-04-13] Verification stamps use ISO 8601 with UTC time: [V 2026-04-13T14:30Z]
[2026-04-13] _test type is a "drain in the floor" — agents who bypass API use it instead of sullying real types.
[2026-04-13] "Manipulating pages by any other method not listed is prohibited" — one line, no map of backdoors.
[2026-04-13] Session records location in wiki needs design — doesn't fit PRD tree, project health, or feedback.

---

## Pending Items

[2026-04-13] Implement document taxonomy in code — update validate.go, UI dropdowns, search filters
[2026-04-13] Complete verification stamps (with timestamps) on all section pages — sections 8, 9 still need element-level stamps with source file refs
[2026-04-13] Prototype project layout — wiki-level template that immature projects can scaffold from
[2026-04-13] Design where session records live in the wiki — not PRD tree, not project health, not feedback
[2026-04-13] Deprecate remaining scattered duplicates (3 UX journals, 2 extra TODO lists) — need legacy API access
[2026-04-13] Migrate existing pages to new type/status values
[2026-04-13] BUG-015: Sidebar shows all projects instead of current project
[2026-04-08] Agent notification/callback system
[2026-04-08] Soft references — vector similarity suggestions
[2026-04-08] OIDC authentication integration
[2026-04-08] Staleness detection — periodic source checksum validation

---

## Session History (most recent first)

### SESSION-2 / 2026-04-13 — Documentation Teardown and Governance
- Deep dive documentation: created 3 architecture pages (system diagram, entry points, internal flows) with Mermaid diagrams
- Created PRD2WIKI Architecture Overview index page linking all 3
- Updated as-built PRD index with architecture deep dive section (sections 7-9)
- Updated REST API page (section 3) — 16→22 endpoints, tree API, blobs, auth
- Updated all 6 original section pages — nav links, accuracy fixes, verification
- Created project health hub: Known Issues (consolidated from 6 pages), TODO (consolidated from 3), Feedback index, Inbox
- Created agent rules for project health pages
- Created "How to Work on This Collection" guide — core rules, page schema, document tree, access methods
- Deprecated 3 old scattered pages (2 bug reports, 1 TODO) with notices
- Added Head/Guide/Parent nav links to all ~28 pages in the collection
- Fixed 13 orphan pages missing nav banners
- Proposed document taxonomy: 10 types (research, spec, plan, report, review, reference, feedback, tracking, project, _test), 7 statuses (sketch→deprecated), artifact state model (3 states + 4 certifications)
- Build process detail: managing agent, builder agents, report types, source-of-truth conflict resolution, backfill manifest
- PHAT-TOAD gap analysis — wiki covers artifact model (6/14 fully), runtime enforcement needs orchestrator
- Steward Orchestrator spec — hierarchical agent coordination using wiki as state, claude-forker for dispatch, claude-proxy for lifecycle
- Verification procedure page — stamp format [V YYYY-MM-DDTHH:MMZ], source file references, 5-step process
- Validated architecture diagram against actual import statements — found and fixed 4 wrong dependency arrows
- Ran parallel verification agents on all 6 section pages — corrected counts, claims, references
- Added BUG-015: sidebar shows all projects
- Gap analysis written to PHAT-TOAD docs directory
- Wiki pages created/updated this session: ~35

### SESSION-1 / 2026-04-07 — 2026-04-08
- Brainstormed and designed full wiki system (spec v0.4, 17 sections)
- Researched standards: MCP, PROV-DM, SLSA, RBAC, Dublin Core, MADR, Gitsign
- Researched prior art: Karpathy LLM Wiki, Pippi Knowledge Librarian, PHAT-TOAD steward framework
- Built all 5 phases: wiki core, librarian, web UI, MCP sidecar, steward agents
- 54 Go source files, 13 packages, all tests passing
- Milkdown WYSIWYG editor bundled via Vite
- LlamaCpp embedder running on Intel GPU (Vulkan) with nomic-embed-text-v1.5
- MCP sidecar connected to Claude Code with 7 tools
- Page history with cross-branch commit tracking and diff view
- Deprecate/restore lifecycle (no delete)
- Fixed: JSON payload mismatch, auto-ID generation, multi-branch index rebuild, branch-agnostic page lookup
- Artifacts: spec (4 versions), research journal, bug journal, bibliography, 5 implementation plans

### Handoff Notes for Next Session

**Priority 1: Verify architecture-overview-system-diagram page**
- Page: /prd2wiki/architecture-overview-system-diagram
- Every element in every diagram needs verification against code imports
- The Mermaid layered architecture diagram was converted from ASCII but may have stale data
- Follow the verification procedure: /prd2wiki/verification-procedure
- Stamp each element with [V YYYY-MM-DDTHH:MMZ] and source files

**Priority 2: Schema migration backfill**
- Plan: /prd2wiki/schema-migration-backfill
- 6 phases of find/replace across all PRD pages
- Old types/statuses still in body text of many pages

**Priority 3: Remaining page verification**
- Sections 8, 9 need element-level verification stamps
- Build flow page needs old status references scrubbed (proposed→submitted, deprecated→retired, feedback type removed)

**Schema in code (final state):**
- Types: research, spec, plan, report, review, reference, tracking, skill, rule, project, _test (+ empty)
- Statuses: draft, submitted, approved, rejected, implemented, completed, superseded, retired
- Feedback type dropped — use report with tag:feedback

**ELK diagram fix:**
- cycleBreakingStrategy: MODEL_ORDER fixes linear flow ordering but breaks complex diagrams
- Current solution: dagre override per-diagram for linear flows, ELK default for complex diagrams
- Global ELK init is clean (no model order overrides) in layout.html

**Wiki API key for this session:** psk_176e81a99553e35eee384450fc030724816175aefb0a1dd2

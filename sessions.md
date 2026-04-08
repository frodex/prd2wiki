# sessions.md — Live Project Context
# Backed up as sessions.md.{SESSION}-{STEP} before each optimize

---

## Project Identity

**Repo:** prd2wiki (`/srv/prd2wiki`)
**Branch:** (not yet initialized)
**What this is:** A system to ingest or create Product Requirements Documents (PRDs) and store them as a wiki. The wiki serves as a living document with full history, decisions, discovery, revision history, authorization audit, and reasoning — all encoded through wiki features. It maintains a published source of approved truth and provides a pipeline to ingest new data from various sources, resolve conflicts, and reason about what gets promoted into the canonical truth.

---

## Active Direction

Brainstorming phase — exploring requirements and architecture for the PRD wiki system.

---

## Operational Conventions

[2026-04-07] Design must use familiar, trusted building blocks. People should look at this project and see something recognizable built from standards they already trust (git, markdown, PRs, RFCs). Only invent where nothing established exists.
[2026-04-07] Architecture must be composable and recursive. New features/uses stem from a robust set of building blocks adapted via configuration, not rewrites. Everything that can be a wiki page should be a wiki page. Like PHAT-TOAD Rule 1: one implementation, adapters absorb differences.

---

## Key Technical Decisions

---

## Pending Items

[2026-04-08] Design agent notification/callback system — subscribe to page changes, wake sleeping agents, notification schema. Sidecar pattern. Connects to steward agent dispatch.
[2026-04-08] Soft references — vector similarity suggestions with accept/dismiss/promote UI
[2026-04-08] OIDC authentication integration
[2026-04-08] Staleness detection — periodic source checksum validation + cascading invalidation

---

## Session History (most recent first)

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

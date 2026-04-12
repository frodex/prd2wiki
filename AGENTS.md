# Agent / development notes

## Canon specification

**Product / implementation spec (authoritative):**

`http://192.168.22.56:8082/projects/default/pages/8634f02`

(Same page on the public host: `https://wiki.droidware.ai/projects/default/pages/8634f02`.)

The **WebFetch / browser fetch tool** blocks private IPs, so it cannot load LAN URLs.

**This machine can:** use the shell to `curl` LAN pages, e.g.  
`curl -sS "http://192.168.22.56:8082/projects/default/pages/8634f02"`  

**Offline mirror (when wiki is stopped):** plan + architecture pages under **`docs/wiki-local/`** (`*.md` body, `*.json` full API). Refresh with **`scripts/fetch-wiki-local.sh`** while the wiki is up.

**Cross-repo (prd2wiki ↔ pippi-librarian):** Read **`docs/constraints-prd2wiki-pippi.md`** before Phase 2, Phase 3a.7 (`libclient` / `syncToLibrarian`), or any code that opens the librarian socket — binding with the Master Plan § Cross-Repo Boundary.

## Wiki base URL (this environment)

When documentation, scripts, or copy refer to **`3200.droidware.ai`**, use the local wiki instead:

**`http://192.168.22.56:8082`**

Treat that as the canonical base URL for browsing, curl checks, and links in this LAN setup.

## Known issues

**Canonical tracker (wiki):** [BUG REPORT: prd2wiki Wiki — Known Issues and Reproduction](http://192.168.22.56:8082/prd2wiki/bug-report-prd2wiki-wiki-known-issues-and-reproduction) — includes open bugs and **LIM-001 / LIM-002** (librarian `memory_search` path: `FindSimilar` fallthrough after filtering, `SearchResult` vs title/snippet, caller contract).

Cross-repo constraints: **`docs/constraints-prd2wiki-pippi.md`**.

## PHAT TOAD — agent conduct (mandatory)

**Source:** `/srv/PHAT-TOAD-with-Trails/steward/system.md` (v0.0.1, draft)

Applies alongside the wiki plan. In practice:

- **Verify before claiming** — Say what is inferred vs run/observed; ask for correction.
- **Constraints first** — Hard limits, anti-patterns, test invariants, performance contracts before architecture; ask *what breaks if touched wrong* when touching another component or repo.
- **No premature “let’s build”** — Scope, interfaces, and constraints resolved for the current slice; cross-node work needs explicit constraint artifacts and comprehension (describe their system; owner corrects).
- **Proposals, not decrees** — Ownership and direction are proposals until you confirm.
- **No shallow “no concerns”** — Walk fragile surfaces against the plan; silence on a known risk is a red flag.
- **PRD discipline** — Unilateral specs are proposals; co-sign where multiple parties are involved. Tag inherited facts `[UNVERIFIED — …]` until verified in this codebase.
- **Complete vs clean** — Handoff docs, constraint updates, and provenance matter as much as passing tests.
- **Review via wiki** — When finishing implementer work tied to a wiki plan, record the handoff **on the wiki** (e.g. `{plan title} IMPLEMENTER-NOTES` or the plan page): commits, scope, verification commands, and explicit “for review” asks — not only in the chat session. Use `PUT /api/projects/{project}/pages/{id}` (see `internal/api/pages.go`) with the wiki base URL above; issue a Bearer key with `go run ./cmd/prd2wiki-keygen -db ./data/index.db` if writes are restricted.

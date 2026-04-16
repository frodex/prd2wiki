# Agent / development notes

## Canon specification

**Product / implementation spec (authoritative):**

`http://192.168.22.56:8082/projects/default/pages/8634f02`

(Same page on the public host: `https://wiki.droidware.ai/projects/default/pages/8634f02`.)

The **WebFetch / browser fetch tool** blocks private IPs, so it cannot load LAN URLs.

**This machine can:** use the shell to `curl` LAN pages, e.g.  
`curl -sS "http://192.168.22.56:8082/projects/default/pages/8634f02"`  

**Offline mirror (when wiki is stopped):** plan + architecture pages under **`docs/wiki-local/`** (`*.md` body, `*.json` full API). Refresh with **`scripts/fetch-wiki-local.sh`** while the wiki is up.

**Wiki-first for agent-facing docs:** Issues, resolutions, and plan trackers that agents must read without a git checkout live on **prd2wiki** (LAN base above). Files under `docs/twowiki/issues/` and similar are **optional pointers** only; do not treat them as the source of truth.

**Cross-repo (prd2wiki ↔ pippi-librarian):** Read **`docs/constraints-prd2wiki-pippi.md`** before Phase 2, Phase 3a.7 (`libclient` / `syncToLibrarian`), or any code that opens the librarian socket — binding with the Master Plan § Cross-Repo Boundary.

## Wiki base URL (this environment)

When documentation, scripts, or copy refer to **`3200.droidware.ai`**, use the local wiki instead:

**`http://192.168.22.56:8082`**

Treat that as the canonical base URL for browsing, curl checks, and links in this LAN setup.

## Fossil (twoWiki) skin — institutional doc (wiki)

Ticket UI for the **twoWiki** Fossil lab is customized via repository **`config`** (TH1 + CSS). **Agents** (including those **without** this repo checkout) must use the **prd2wiki** page **[twoWiki Fossil skin — implementation notes (agents)](http://192.168.22.56:8082/prd2wiki/twowiki-fossil-skin-implementation-notes)** as the canonical source for behavior (**post-submit `redirect`**, **`/ticket/` → `/tktview`**, TH1 constraints, verification). Public mirror: `https://wiki.droidware.ai/prd2wiki/twowiki-fossil-skin-implementation-notes`.

Repo **`vendor/twowiki-fossil-skin/README.md`** is only a **file map + apply commands**; it points at that wiki page for institutional detail.

**MANDATORY — twoWiki vs prd2wiki (do not forget):** **twoWiki** work is **only** the Fossil bench: files under **`vendor/twowiki-fossil-skin/`**, SQL into the **`.fossil` `config`** table, and ticket/repo ops on the **Fossil host** (dashboard, `fossil sql`, `fossil ticket`, JSON ticket API when enabled). The **`vendor/`** path is the cue: packaged skin overlay for Fossil, **not** the prd2wiki Go app. **Do not** change **`internal/web/`** (prd2wiki’s own wiki UI) for twoWiki chrome, colors, nav, or breadcrumbs unless the user **explicitly** asks to modify **prd2wiki** — that would be like changing twoWiki’s background by editing SQLite because Fossil uses it. This skin process has been applied before; stay on the **same** Fossil apply path unless told otherwise.

**twoWiki bench — editing tickets (agents have multiple paths):** On the LAN lab host, tickets in `/opt/twowiki/repo.fossil` can be updated by **(1)** SSH + `fossil ticket change|set UUID … --quote … -R /opt/twowiki/repo.fossil` (what we used for the sortable-matrix fix), **(2)** the **JSON API** when the server is built with `--json` — `POST /json/ticket/save` (see `vendor/fossil-json-ticket/README.md`; auth + Referer rules apply), or **(3)** the normal **human web UI** (`/tktedit/…`) in a browser. Prefer (1) or (2) for scripted, verifiable edits; use (3) when validating UX or when API/SSH is unavailable.

**Bench issue TWOWIKI-001** (sortable tables / `gt` artifacts on ticket view): resolution and checklist on wiki — [/projects/default/pages/bb219262-74c8-4a92-8379-9b3132227398](http://192.168.22.56:8082/projects/default/pages/bb219262-74c8-4a92-8379-9b3132227398); plan tracker — [/projects/default/pages/3436fe3](http://192.168.22.56:8082/projects/default/pages/3436fe3).

**Today:** twoWiki is **qualification / bench** only. **prd2wiki** is the source of truth for real project pages, trackers, and agent-facing docs.

**Roadmap (not in effect yet):** the team may later track some projects **in both** systems, with **twoWiki as a live A/B** against prd2wiki, once the twoWiki **feature set** is far enough along. Until that is explicitly announced and documented on the wiki, assume **no dual-write** and **no parity obligation** between the two surfaces.

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

## MCP sidecar environment variables

When running `prd2wiki-mcp` (the MCP stdio sidecar), these environment variables configure its behavior:

| Variable | Required | Description |
|----------|----------|-------------|
| `PRDWIKI_API_URL` | optional | Base URL for the wiki HTTP API (default `http://localhost:8080`) |
| `PRDWIKI_TREE_ROOT` | recommended | Path to tree directory; enables tree-path tools |
| `PRDWIKI_DATA_DIR` | with tree | Path to data directory; required when `PRDWIKI_TREE_ROOT` is set |
| `PRDWIKI_API_TOKEN` | optional | Bearer token for authenticated write operations; when set, MCP mutating requests (create, delete) include `Authorization: Bearer` header |

**`PRDWIKI_API_TOKEN`:** All write mutations now require authentication when the key store is configured. Set this to a write-scoped service key so the MCP sidecar can create and delete pages. Generate a key with:

```bash
go run -mod=mod ./cmd/prd2wiki-keygen -db ./data/index.db
```

**Never commit raw keys** to the repo or embed them in wiki page bodies.

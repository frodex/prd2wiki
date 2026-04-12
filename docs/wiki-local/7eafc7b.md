# BLOCKERS: prd2wiki Master Plan — Audit Round 13 (Phase 3 expansion deep-dive)

**Date:** 2026-04-12  
**Against:** [Master Implementation Plan 8634f02](/projects/default/pages/8634f02), [Phase 3 Expansion 56803d5](/projects/default/pages/56803d5), [Unified Identity de836ff](/projects/default/pages/de836ff), [File Map 97a0970](/projects/default/pages/97a0970), [Librarian Tools 6ccd407](/projects/default/pages/6ccd407), [Version-Aware Memory c6525ac](/projects/default/pages/c6525ac), [Export/Import cec9acb](/projects/default/pages/cec9acb), [Pre-flight 92657c7](/projects/default/pages/92657c7).

**Method:** Full-text review of **8634f02** + **56803d5** with cross-check against identity, file map, librarian tools, memory design, export, and pre-flight. This round validates the **Phase 3a–3d** decomposition and schedule (~61h total) after the master plan expansion — not a repeat of R1–R12 P0 hunts.

**Verdict:** The **scope expansion is sound** (router + CRUD + blobs + UI + management + admin dashboards was materially underestimated at ~8h). **No P0 canon contradictions** were found between **56803d5** and **de836ff** / **c6525ac** on tree operations and volatile `.link` line 2. **New P1/P2 gaps** are **implementation hazards** and **documentation completeness** — listed below.

---

## P1 — `internal/libclient/` missing from Phase 3a bullets on [8634f02](/projects/default/pages/8634f02)

Phase **3a** describes router, CRUD, API, blobs, and `syncToLibrarian` on edit — but does **not** explicitly name **`internal/libclient/`** (remote librarian client) as in prior plan + [97a0970](/projects/default/pages/97a0970). Implementers can wire the tree **before** the client package, breaking the edit path.

**Fix:** Add one line under Phase 3a: wire **`libclient`** + **`syncToLibrarian`** per existing contract.

---

## P1 — API migration: `302` is unsafe for non-GET clients ([56803d5](/projects/default/pages/56803d5) §3a.5)

The doc says old `/api/projects/...` → **302** to `/api/tree/...`. Browsers follow GET redirects; **POST/PUT/PATCH** clients often **do not** replay correctly on 302. Risk: silent breakage during transition.

**Fix:** Specify **307/308**, parallel routes + deprecation window, or versioned API — pick one and document.

---

## P1 — Catch-all tree router needs **reserved path** table ([56803d5](/projects/default/pages/56803d5) §3a.2)

`GET /{tree-path}` must not capture **`/api/**`, `/static/**`, `/blobs/**`, `/admin/**`, legacy `/projects/**`, or health/debug routes. Without an explicit prefix denylist, the tree router will **steal traffic** from APIs and admin.

**Fix:** Add a short “reserved prefixes” subsection (bullet list).

---

## P1 — Phase **3d** vs Phase **5** ordering ([8634f02](/projects/default/pages/8634f02) dependencies)

The graph runs **3d (admin UI)** → **4** → **5 (export/import CLIs)**. **56803d5** §3d says admin pages **invoke `prd2wiki-export` / import / verify`** in the background. Those binaries are **Phase 5** deliverables.

**Fix:** Either (a) **stub 3d** until CLIs exist, (b) **implement Phase 5 CLIs before 3d**, or (c) **reorder** the dependency graph. One explicit sentence in the master plan removes schedule ambiguity.

---

## P1 — Wiki delete: `.link` removed, librarian rows “orphaned” ([56803d5](/projects/default/pages/56803d5) §3a.4)

[de836ff](/projects/default/pages/de836ff) already allows delete = tree-only with git preserved. The **gap** is **product semantics**: does search still return librarian hits for removed pages? Steady-state “orphan” may be wrong if **compactor** or **`memory_delete` by `page_uuid`** is expected.

**Fix:** State intended behavior: **async tombstone**, **explicit librarian delete**, or **acceptable orphan until compaction** — and align with [c6525ac](/projects/default/pages/c6525ac) delete/MCP story.

---

## P1 — Two MCP surfaces: **wiki_*** vs **`memory_*`** ([56803d5](/projects/default/pages/56803d5) §3c.5 vs [6ccd407](/projects/default/pages/6ccd407))

[6ccd407](/projects/default/pages/6ccd407) defines **four pippi Librarian tools** (`memory_*`) — R12 schema gate applies there. **56803d5** updates **seven prd2wiki `wiki_*` tools** + **`wiki_move` / `wiki_rename`**. These are **different codebases and schemas**.

**Fix:** Add a sentence to Phase 1 / Phase 3c: **two** tool-schema tracks — **Librarian** (namespace, `page_uuid`, `record_id`) vs **Wiki MCP** (tree paths, move/rename). Prevents Phase 1 from merging them accidentally.

---

## P2 — [97a0970](/projects/default/pages/97a0970) file map drift

No `api/tree`, `/admin/export|import|verify`, or new router layout appears in a quick consistency pass. **Implementation will diverge** from the “what lives where” doc unless updated when **3a** starts.

**Fix:** Update file map when router/API paths land.

---

## P2 — Status mismatch: [8634f02](/projects/default/pages/8634f02) “ready” vs [56803d5](/projects/default/pages/56803d5) “Draft — needs review”

Not illogical (parent approved, child refining) but signals **56803d5** as the **detailed** Phase 3 spec — **finalize or explicitly mark** which sections are binding before build.

---

## P2 — Phase 4 effort (~1h) vs expanded surface

Stress + deploy after **61h** of change is **optimistic**. Optional bump or split “smoke” vs “hard soak.”

---

## Credit — what the expansion gets right

- **3a→3d ordering** inside [56803d5](/projects/default/pages/56803d5) matches dependencies (scanner/router before UI before tools before admin).
- **Blob store** + migration hook to pre-flight aligns with identity + [cec9acb](/projects/default/pages/cec9acb) bundle story.
- **Legacy URL → tree** preserves audit-era migration goals.
- **~29h Phase 3** in [56803d5](/projects/default/pages/56803d5) explains the **~40h → ~61h** master plan delta honestly.

---

## Summary

| ID | Severity | Topic |
|----|----------|--------|
| R13-1 | P1 | Explicit **`libclient`** in Phase 3a ([8634f02](/projects/default/pages/8634f02)) |
| R13-2 | P1 | API redirect **semantics** (302 vs 307/308 / parallel routes) ([56803d5](/projects/default/pages/56803d5)) |
| R13-3 | P1 | **Reserved URL prefixes** for tree router ([56803d5](/projects/default/pages/56803d5)) |
| R13-4 | P1 | **3d vs 5** dependency — admin UI vs CLI existence ([8634f02](/projects/default/pages/8634f02)) |
| R13-5 | P1 | **Delete page** vs librarian **orphan** policy ([56803d5](/projects/default/pages/56803d5), [de836ff](/projects/default/pages/de836ff)) |
| R13-6 | P1 | **Wiki MCP** vs **Librarian `memory_*`** schema tracks ([56803d5](/projects/default/pages/56803d5), [6ccd407](/projects/default/pages/6ccd407)) |
| R13-7 | P2 | [97a0970](/projects/default/pages/97a0970) update for new paths |
| R13-8 | P2 | [56803d5](/projects/default/pages/56803d5) draft vs ready |
| R13-9 | P2 | Phase **4** duration realism |

**Housekeeping:** Add **Round 13** (this page) to the [8634f02](/projects/default/pages/8634f02) audit line after publish.

**Next round:** After [56803d5](/projects/default/pages/56803d5) addresses R13-1–R13-6, Round 14 can be a **short verification pass** or **schema.d** diff only.


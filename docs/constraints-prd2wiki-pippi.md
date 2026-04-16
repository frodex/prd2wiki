# Constraint declaration: prd2wiki ↔ pippi-librarian

**Status:** Binding for any change that crosses this boundary (socket, tickets, `memory_*`, `libclient`, `syncToLibrarian`).  
**Canon (wiki):** [Master Plan 8634f02](http://192.168.22.56:8082/projects/default/pages/8634f02), [Version-Aware Memory c6525ac](http://192.168.22.56:8082/projects/default/pages/c6525ac), [Librarian Tools 6ccd407](http://192.168.22.56:8082/projects/default/pages/6ccd407), [Unified Identity de836ff](http://192.168.22.56:8082/projects/default/pages/de836ff).  
**Past failure analysis:** [Head-Delete Gap 13c87ad](http://192.168.22.56:8082/projects/default/pages/13c87ad); audits [R1–R13](http://192.168.22.56:8082/projects/default/pages/7eafc7b).

---

## 1. Scope

- **In scope:** Wire **prd2wiki** to **pippi-librarian** via **`internal/libclient/`**, **`syncToLibrarian`**, and the **four `memory_*`** tools contract.
- **Out of scope for a given PR:** If the change does **not** open a socket, send tickets, or call memory APIs, this file does not block — use **prd2wiki-only** constraints for that chunk.

---

## 2. Librarian integration posture (avoid “leakage”)

Treat **pippi-librarian** as a **separate codebase** that we depend on and control—**not** as a place to push prd2wiki-specific shortcuts. The useful analogy is **SQLite**: you do not fork or rewrite SQLite because your application wants a slightly different API shape; you fix **your** usage and your integration layer.

| Do here (prd2wiki) | Do there (librarian) — sparingly |
|--------------------|----------------------------------|
| Fix **call sites**: `libclient`, HTTP/MCP handlers, tree orchestration, correct use of **`Librarian.Submit`**, **`RemoveFromIndexes`**, and other **published** APIs. | Fix **bugs**, **security**, or **correctness** issues that are genuinely **in** librarian. |
| If cleanup or sync is wrong because we **bypassed** the librarian API (e.g. talked to SQLite or index only), fix **prd2wiki** to use the **intended** orchestration path. | Add **features** when they are justified as **librarian** capabilities with a clear contract for **all** legitimate callers—not “prd2wiki needs this, so we’ll implement it in librarian.” |

**Anti-pattern:** “It’s easier to add a special function or branch in **librarian** so prd2wiki can wire X in fewer steps—so we’ll do it there.” That is **leakage**: prd2wiki concerns belong in prd2wiki unless the change is a real librarian product or protocol improvement.

---

## 3. Non-negotiable semantics

| Topic | Rule |
|--------|------|
| **Stable ID** | **`page_uuid`** (from `.link` line 1, frontmatter `id`, git filename). Never derive memory identity from slug or tree path alone. |
| **Volatile ID** | Head **`mem_`** changes every version write (new-chain-forward). **`.link` line 2** updated after successful sync. |
| **Namespace** | `wiki:{project-uuid}` (full project UUID string unless canon explicitly defines a shorter form — match [6ccd407](http://192.168.22.56:8082/projects/default/pages/6ccd407)). |
| **Entities** | `page_uuid == ""` for non-wiki rows — **not NULL**. |
| **Writes** | **New head first**, then demote old head — **no** Delete+Add on the **new** head row. **Embed before any Lance write** (single goroutine critical section). |
| **Store return** | `Store()` → **`(recordID string, err error)`**; MCP adds **`record_id`** (backward compatible). |
| **Delete (wiki removes `.link`)** | Tree delete must eventually reconcile librarian (e.g. async delete by `page_uuid` or documented orphan + compactor). **Silent orphan search hits** are not acceptable as steady state. |

---

## 4. Transport and auth (must not regress)

| Topic | Rule |
|--------|------|
| **Channel** | Unix socket (config: `librarian.socket` or equivalent). |
| **Auth** | Ticket-based auth as implemented in pippi — **no** plaintext secrets in URLs or export bundles. |
| **Failure** | If librarian unavailable: **git save still succeeds**; queue retry per plan (bounded, deduped). **Never** block user edit on librarian up. |
| **Timeouts** | Client calls use bounded contexts (e.g. ~30s for sync) — align with existing plan snippets. |

---

## 5. What must not break

- **pippi-librarian:** Existing **MCP / HTTP** entrypoints used by **other** clients (if any) must remain working, or changes must be **explicitly versioned** and documented.
- **prd2wiki:** Page submit to **git** and **SQLite/index** paths must remain correct when `PageUUID == ""` during migration (sync no-op).
- **Two tool worlds:** **Librarian `memory_*`** (pippi) vs **Wiki `wiki_*`** (prd2wiki) — **separate schemas**; wiki may call librarian internally but **must not** merge tool definitions into one schema by accident.

---

## 6. Tests that must stay green before merge (minimum)

**prd2wiki**

- `go test ./...` (and **`-race`** where the project already uses it for CI).
- Any existing **`internal/librarian`** / **`internal/mcp`** tests touching submit or MCP.

**pippi-librarian**

- `go test ./...` for packages touched.
- **Integration:** Any new **cross-repo** slice should add at least **one** test or scripted check that exercises **socket + ticket + one memory call** (can be behind build tag if needed).

**Cross-repo (when boundary code lands)**

- **Smoke:** Start librarian + wiki (or test harness), **one** successful `memory_store` round-trip for a synthetic `page_uuid`, assert **new `mem_`** returned and **retry** path exercised on forced failure (optional but ideal).

---

## 7. Known failure modes (do not repeat)

| Failure | Reference |
|---------|-----------|
| Treating Lance **upsert** as atomic batch | R3–R4; use **ordered writes** + repair per **c6525ac**. |
| **Head row** vanishing on crash | [13c87ad](http://192.168.22.56:8082/projects/default/pages/13c87ad); **new-chain-forward** only. |
| **Lex order** on unpadded base-36 IDs | R8; **zero-pad** timestamp segment to fixed width. |
| **Stable `mem_` in MCP** after new-chain-forward | R9; **`record_id`** volatile; **`page_uuid`** for “current”. |
| **Export/import** tree vs librarian **line 2** mismatch | R10; **rewrite `.link` line 2** after import; verify **skips** volatile line. |

---

## 8. Change control

- Any change to **socket path**, **ticket format**, **`memory_*` JSON schema**, or **Store signatures** requires **updating this file** + **wiki tool page [6ccd407](http://192.168.22.56:8082/projects/default/pages/6ccd407)** (or a linked schema) in the **same** merge window.

---

## 9. Revision

| Date | Change |
|------|--------|
| 2026-04-12 | Initial constraint declaration for cross-repo implementation. |
| 2026-04-15 | §2 Librarian integration posture (SQLite analogy; avoid leakage into librarian). |
| 2026-04-15 | §10 Implementer planning note (status vs implementation plan). |

---

## 10. Implementer note — understanding, agreement, and planning gate

**Understanding:** This file binds **cross-repo** work (prd2wiki ↔ pippi-librarian). **§2** states how we treat librarian: as a **dependency** with a **published** API—fix **our** integration and call paths first; change **librarian** only for genuine flaws or librarian-scope features, not for prd2wiki convenience.

**Agreement:** That posture matches how implementers should execute **Phase 3 / write-core** decisions that touch sync and cleanup (e.g. using **`RemoveFromIndexes`** where appropriate): **caller and orchestration fixes in prd2wiki**, not ad hoc librarian rewrites.

**Status (not “done”):** The **planner** is still working **open items on their side** (process, wiki plan alignment, owner gates, and any issues raised in design-brief review). **We do not consider the overall Phase 3 picture resolved enough** to treat it as a closed design: remaining gaps should be **closed in the wiki plan / design brief** before coding proceeds as the only source of truth.

**Gate:** Cross-boundary and write-core work should move into a **dedicated implementation plan phase** **after** those planner-side issues are cleared and the implementer’s concerns are **explicitly addressed or rejected with rationale** on the plan. Until then, treat Phase 3 as **design-in-progress**, not approved implementation scope.

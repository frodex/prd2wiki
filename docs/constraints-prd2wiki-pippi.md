# Constraint declaration: prd2wiki ↔ pippi-librarian

**Status:** Binding for any change that crosses this boundary (socket, tickets, `memory_*`, `libclient`, `syncToLibrarian`).  
**Canon (wiki):** [Master Plan 8634f02](http://192.168.22.56:8082/projects/default/pages/8634f02), [Version-Aware Memory c6525ac](http://192.168.22.56:8082/projects/default/pages/c6525ac), [Librarian Tools 6ccd407](http://192.168.22.56:8082/projects/default/pages/6ccd407), [Unified Identity de836ff](http://192.168.22.56:8082/projects/default/pages/de836ff).  
**Past failure analysis:** [Head-Delete Gap 13c87ad](http://192.168.22.56:8082/projects/default/pages/13c87ad); audits [R1–R13](http://192.168.22.56:8082/projects/default/pages/7eafc7b).

---

## 1. Scope

- **In scope:** Wire **prd2wiki** to **pippi-librarian** via **`internal/libclient/`**, **`syncToLibrarian`**, and the **four `memory_*`** tools contract.
- **Out of scope for a given PR:** If the change does **not** open a socket, send tickets, or call memory APIs, this file does not block — use **prd2wiki-only** constraints for that chunk.

---

## 2. Non-negotiable semantics

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

## 3. Transport and auth (must not regress)

| Topic | Rule |
|--------|------|
| **Channel** | Unix socket (config: `librarian.socket` or equivalent). |
| **Auth** | Ticket-based auth as implemented in pippi — **no** plaintext secrets in URLs or export bundles. |
| **Failure** | If librarian unavailable: **git save still succeeds**; queue retry per plan (bounded, deduped). **Never** block user edit on librarian up. |
| **Timeouts** | Client calls use bounded contexts (e.g. ~30s for sync) — align with existing plan snippets. |

---

## 4. What must not break

- **pippi-librarian:** Existing **MCP / HTTP** entrypoints used by **other** clients (if any) must remain working, or changes must be **explicitly versioned** and documented.
- **prd2wiki:** Page submit to **git** and **SQLite/index** paths must remain correct when `PageUUID == ""` during migration (sync no-op).
- **Two tool worlds:** **Librarian `memory_*`** (pippi) vs **Wiki `wiki_*`** (prd2wiki) — **separate schemas**; wiki may call librarian internally but **must not** merge tool definitions into one schema by accident.

---

## 5. Tests that must stay green before merge (minimum)

**prd2wiki**

- `go test ./...` (and **`-race`** where the project already uses it for CI).
- Any existing **`internal/librarian`** / **`internal/mcp`** tests touching submit or MCP.

**pippi-librarian**

- `go test ./...` for packages touched.
- **Integration:** Any new **cross-repo** slice should add at least **one** test or scripted check that exercises **socket + ticket + one memory call** (can be behind build tag if needed).

**Cross-repo (when boundary code lands)**

- **Smoke:** Start librarian + wiki (or test harness), **one** successful `memory_store` round-trip for a synthetic `page_uuid`, assert **new `mem_`** returned and **retry** path exercised on forced failure (optional but ideal).

---

## 6. Known failure modes (do not repeat)

| Failure | Reference |
|---------|-----------|
| Treating Lance **upsert** as atomic batch | R3–R4; use **ordered writes** + repair per **c6525ac**. |
| **Head row** vanishing on crash | [13c87ad](http://192.168.22.56:8082/projects/default/pages/13c87ad); **new-chain-forward** only. |
| **Lex order** on unpadded base-36 IDs | R8; **zero-pad** timestamp segment to fixed width. |
| **Stable `mem_` in MCP** after new-chain-forward | R9; **`record_id`** volatile; **`page_uuid`** for “current”. |
| **Export/import** tree vs librarian **line 2** mismatch | R10; **rewrite `.link` line 2** after import; verify **skips** volatile line. |

---

## 7. Change control

- Any change to **socket path**, **ticket format**, **`memory_*` JSON schema**, or **Store signatures** requires **updating this file** + **wiki tool page [6ccd407](http://192.168.22.56:8082/projects/default/pages/6ccd407)** (or a linked schema) in the **same** merge window.

---

## 8. Revision

| Date | Change |
|------|--------|
| 2026-04-12 | Initial constraint declaration for cross-repo implementation. |

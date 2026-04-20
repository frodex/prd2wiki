# BLOCKERS: Master Implementation Plan — Pre-Implementation Audit

**Date:** 2026-04-11  
**Status:** DRAFT — **resolve before writing implementation code**  
**Scope:** Full cross-review of [prd2wiki Master Implementation Plan](/projects/default/pages/8634f02) (8634f02) and all documents it depends on or supersedes. Assumption: **every phase ships**; anything that reads like a temporary compromise must be upgraded to a forward-looking decision **now**.

**Related plans (read together):** [8634f02](/projects/default/pages/8634f02), [Pre-flight 92657c7](/projects/default/pages/92657c7), [File Map 97a0970](/projects/default/pages/97a0970), [Unified Identity de836ff](/projects/default/pages/de836ff), [Version-Aware Memory c6525ac](/projects/default/pages/c6525ac), [Export/Import cec9acb](/projects/default/pages/cec9acb).

---

## Executive summary

| Severity | Count | Meaning |
|----------|-------|---------|
| **P0** | 12 | Contradictory identity/API/table story; master “architecture” pages still **REVIEW** with open decisions; parallel-phase claims that would merge conflicting edits on the same files. |
| **P1** | 18 | Stale downstream pages; ordering mismatches; speculative numbers; unresolved cutover for SQLite/vectors/URLs. |
| **P2** | 9 | Documentation hygiene (naming, examples, tool count text). |

**Bottom line:** Do **not** treat 8634f02 as internally consistent until the **P0** items have single authoritative answers edited into the canon pages (8634f02, c6525ac, de836ff, 92657c7, 97a0970).

---

## P0 — Hard blockers (logic, ordering, or identity)

### P0-1: Version identity story is forked (8634f02 vs c6525ac)

- **8634f02 § Gap 2** still says: `Version key = {page-uuid}:v{N}`.
- **c6525ac** (current) explicitly **supersedes** suffix schemes (`:v{N}`) and requires **`mem_` generated IDs** with chains linked by record IDs + `page_uuid` / `superseded_by`.

These cannot both be true. **Resolution:** Delete or rewrite Gap 2 in 8634f02 to match c6525ac only. If any engineer implements `:v{N}` in Lance `id`, they will fight the merged entity/memory table and the “IDs never reused” invariant in c6525ac §Invariants.

### P0-2: `memory_store` calling convention is forked (8634f02 vs c6525ac)

- **8634f02 `syncToPippi`** calls something shaped like `MemoryStore(ctx, "wiki:"+ProjectUUID, Frontmatter.ID, body, ext)` — **namespace + legacy key + content**.
- **c6525ac § MCP Tools** shows `memory_store` with **`page_uuid`** as the logical key and de836ff says page identity is the **UUID in `.link` line 1**.

Until **Frontmatter.ID** is provably identical to **page UUID** for every page after migration, this is undefined behavior. **Resolution:** One canonical contract:

- Either **`key` = page UUID string everywhere** (and frontmatter `id` is migrated to UUID), or
- Explicit dual-phase API with a **compat shim** that is **deleted** in the same milestone (no lingering “sometimes hash sometimes UUID”).

### P0-3: “Stable head `mem_` ID” vs versioning write path (c6525ac internal)

c6525ac **Write** diagram says: on update, **copy old content to a new superseded record (new ID)** and **update current in place (same `mem_` ID)**. Elsewhere the doc ecosystem still describes “every version is a new row” without emphasizing **head ID stability**. Implementation teams will argue whether **Upsert** in Lance (delete-by-`id` + add) still targets the **same** head row.

**Resolution:** Add one **sequence diagram** to c6525ac: Lance upsert row for head ID, new row insert for superseded snapshot, and how `page_uuid → current head id` index updates.

### P0-4: Phase parallelism claim vs file ownership (8634f02 Dependencies)

8634f02 says **Phase 1 can run in parallel with Pre-flight B**, but **Pre-flight A Item 6** and **Phase 1** both modify **`memory_lance.go`, `memory.go`, `entities.go`, `main.go`**.

**Resolution:** Serialize **pippi-librarian** changes: **Pre-flight A items 6–7 complete and merged** *before* Phase 1 schema/version columns land, **or** a single combined branch with a joint schema (record_type + ext_json + version columns) to avoid thrash.

### P0-5: Pre-flight B item order disagrees between docs

- **92657c7** execution order: Item **8 → 10 → 9 → 11** (explicitly: rename repos before `.link` creation in the narrative).
- **8634f02** pre-flight table lists Item **9 before 10**.

Creating `.link` files may depend on knowing **final repo paths** and stable git layout after rename. **Resolution:** Pick **one** ordered checklist; update the other page to match.

### P0-6: Master identity doc is not closed

**de836ff** is marked **REVIEW — open questions** (UUID vs ULID, legacy URL strategy, flat `pages/{uuid}.md` vs hash-prefix, `.access`, etc.). 8634f02 imports de836ff as **THE master design**.

You cannot implement tree migration + git layout + routing while those are open. **Resolution:** Either move open items to a **Phase 6** explicitly out of scope, or **decide** and edit de836ff to **Status: locked** with defaults.

### P0-7: `memory_delete` / promote-previous semantics vs implementation surface

c6525ac specifies **delete current → promote previous** and complex ghost/tail rules. **8634f02 Phase 2.7** mentions MCP handlers for `-chain`/`-history` but the **wiki schema YAML** in 8634f02 only extends **store/search/get** — **no `memory_delete` in the schema extension block**.

**Resolution:** Extend `schema.d/wiki.yaml` (and registrar merge rules) to include **delete** tool extensions **or** explicitly state delete is **not** wiki-extended (core only) — but then success criteria must not claim agent-discoverable parity.

### P0-8: Table / directory naming fork (`pippi_memory` vs `pippi_records`)

8634f02 Gap 3 says **one table `pippi_records`**. Code + file map use **`pippi_memory`** constant and `pippi_memory.lance/` path. **Resolution:** Rename consistently (code constant + docs + on-disk dir) **or** call it “logical `pippi_records` table inside existing Lance DB” without renaming files — pick one sentence used everywhere.

### P0-9: Phase 2 field names drift (`is_current` vs `version_status`)

8634f02 Phase 2 step table lists **`is_current`**. c6525ac and the memory design use **`version_status`**. **Resolution:** single field name everywhere (JSON, Arrow, Go struct).

### P0-10: Gap 1 retry queue vs sync implementation

Gap 1 says bounded queue stores **“key + metadata only”**. The shown **`syncToPippi`** still ships **full page body** in a goroutine on failure paths; a queue that only stores metadata **cannot** retry full re-index without re-reading git — which is fine, but then **do not** claim the queue replays content. **Resolution:** Specify queue entries: **git ref + path + project UUID** sufficient to re-read, or **persist nothing** and rely on periodic full reconcile.

### P0-11: Indexer / SQLite / URL routing cutover is unspecified

File map + cec9acb say **SQLite FTS and `vectors/pages.json` go away** in favor of tree + librarian. 8634f02 Phase 3 replaces list/sidebar paths but does not define **API resolve by ID** during migration when some pages are hash-ID and some UUID, nor **search** behavior if SQLite is removed before librarian parity.

**Resolution:** Add a **cutover matrix**: which features work in each milestone; which routes return **301** to legacy URLs; when `index.db` may be deleted.

### P0-12: Pre-flight Item 9 depends on “search librarian” for line 2 of `.link`

Item 9 says get librarian record ID by **searching librarian** — during initial migration the librarian may be **empty** or keyed differently. **Resolution:** `.link` line 2 **empty on day one**, filled on first successful `syncToPippi` (matches de836ff example flow). Remove circular dependency language.

---

## P1 — Cross-document conflicts and deprecation creep

### Satellite pages that lag the unified story

| Page | Risk |
|------|------|
| [6ccd407](/projects/default/pages/6ccd407) | Examples use **`wiki-pages`**, path-like **keys**, old `Match` shape. Will mislead agents unless header says **superseded by de836ff + c6525ac** or page is rewritten. |
| [ac28f83](/projects/default/pages/ac28f83) | Old “~4.5h” implementation plan; **GitCommit on MemoryRecord** core fields vs **ext_json** in 8634f02; **3-tool** story. |
| Older appendices on 8634f02 | If any section still says **3 tools**, **suffix IDs**, or **HTTP 9090** without tickets — strip or label **obsolete**. |

### Namespace strings

- de836ff / 8634f02: `wiki:{project-uuid}`.
- 6ccd407: `wiki` or `wiki-pages`.
- c6525ac MCP examples: sometimes omit **namespace** entirely in `memory_store`.

**Resolution:** one **canonical namespace** string and one **JSON field** name in tool schemas (`namespace` vs embedding in page_uuid only).

### Tool count

c6525ac locks **4 tools** (adds **delete**). 8634f02 success criteria still list **“3 tools: store, search, get”** in places (if present in checklist). **Resolution:** global find/replace in canon docs.

### `source_repo` vs new repo layout

Wiki extension schema still speaks in **“git repo”** terms; sync uses `proj_{uuid8}.git`. Filters and human docs must use the **same** string format.

### Stress test numbers

“466 pages”, “100 concurrent searches” — tie to **manifest** (`cec9acb`) or mark **example targets**, not contractual gates.

### CGO / `lib/linux_amd64`

Multiple docs assume path exists; some workspaces lack `lib/`. **Resolution:** “required artifact on build agents” or vendor libs — don’t assume `/srv/.../lib` in CI.

---

## P2 — Documentation hygiene (still fix before coding)

- **Phase 5** export includes `schema/` — ensure **which** schema (prd2wiki vs pippi-librarian) and **service keys never** in tar (cec9acb says keys not exported — repeat in 8634f02).
- **EmbeddingText** + extensions: Phase 1.7 must be consistent with **entity** `record_type` rows (don't apply wiki-only semantic rules to entities).
- **Contested tools / entry tools**: file map says NO CHANGE — verify they still make sense when memory API gains **page_uuid**-first addressing.

---

## Security & abuse (forward-looking)

1. **Ticket + nonce** model is correct for local Unix; ensure **prd2wiki** UID is the only peer that can obtain tickets with `disclose:content` for production data.
2. **Filter injection** — c6525ac says IDs only from our tools; hold that line when exposing delete.
3. **Retry queue** — if it ever stores **content**, define **disk encryption** or **RAM-only** + max size; align with P0-10.
4. **Export bundle** — manifest must **exclude** secrets; verify **deep verify** doesn’t diff Badger keys.

---

## Required resolutions checklist (sign-off)

- [ ] **P0-1–P0-3:** Single ID + key + `memory_store` contract (8634f02 ⇄ c6525ac ⇄ de836ff).
- [ ] **P0-4–P0-5:** Ordered pre-flight B + no unsafe parallel merges on `memory_lance.go`.
- [ ] **P0-6:** de836ff decisions closed or explicitly deferred with phase numbers.
- [ ] **P0-7–P0-9:** Tool schema + field names + table naming unified.
- [ ] **P0-10–P0-12:** Retry + migration + `.link` bootstrap story coherent.
- [ ] **P1:** Satellite pages marked obsolete or updated.
- [ ] **Cutover matrix** for SQLite, vectors, URLs, API (P0-11).

---

## Notes to editors

This page is intentionally **harsh**: the underlying vision is strong, but the documentation set evolved across multiple rewrites. **Deprecation creep** is visible (suffix IDs vs `mem_`, 3 vs 4 tools, namespace variants). Treat this list as **merge conflicts** in prose form — resolve them **before** paying implementation cost.

**Source:** Static review of wiki exports on 2026-04-11; spot-checked `/srv/pippi-librarian/internal/librarian/memory_lance.go` (table name `pippi_memory`, `id` string column, no DB-generated UID).


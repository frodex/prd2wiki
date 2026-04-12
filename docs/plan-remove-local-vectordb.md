# PLAN: Remove Wiki's Local Vector Store — Use Librarian for All Search

**Date:** 2026-04-12 (revised after code verification review)
**Status:** DRAFT — REQUIRES REVIEW BEFORE IMPLEMENTATION
**Priority:** Critical
**Author:** Claude Opus 4.6
**Reviews (corrections from all four incorporated below):**
- `plan-remove-local-vectordb-RESPONSE-verification.md` — first verification: dead dedup, API merge behavior, content prefixing
- `plan-remove-local-vectordb-REVIEW-2-deep.md` — deep review: TextChunk dependency, prd2wiki-import absent, web≠API search, local scoring formula
- `plan-remove-local-vectordb-REVIEW-3-verification.md` — third pass: Librarian has no Searcher (FTS fallback in handlers not Librarian), search logging is new work, grep patterns incomplete
- `plan-remove-local-vectordb-REVIEW-4-in-chat.md` — fourth pass: namespace mapping (repo key vs wiki:uuid), vocabulary normalization parity, write latency table correction
- `plan-remove-local-vectordb-REVIEW-5-stepped-dual-repo.md` — fifth pass (DUAL REPO): memory_delete takes id not page_uuid (BLOCKER), Step 4 contradicts Part 5 Option B, vocab normalization differs between repos, firstLineTitle vs frontmatter title

**Reviewer instructions:** This plan must be examined for anything missing that would prevent the intended outcome. If a step, dependency, or integration point is missing, flag it as a BLOCKER. Every claim has been verified against source code as of 2026-04-12. Function names are used as anchors (not line numbers, which drift).

### Codebase scope (required)

Reviews and verification passes are **invalid for comparison** unless they state **exactly** which trees were read. Different clones, branches, or repos produce different conclusions.

**Requirement — every review MUST include a table like this (fill in at review time):**

| Field | Value |
|--------|--------|
| **Wiki / prd2wiki root** | `/srv/prd2wiki` |
| **Remote** | `origin` → `https://github.com/frodex/prd2wiki.git` (local is ahead of remote) |
| **Branch** | `main` |
| **Commit** | run `git rev-parse HEAD` at review time |
| **Librarian / pippi-librarian root** | `/srv/pippi-librarian` — branch `main` — commit `cc96c4b` |
| **Librarian key files for review** | `cmd/pippi-librarian/main.go` (MCP handlers), `internal/librarian/memory.go` (`StoreWiki`), `internal/librarian/memory_lance.go` (Arrow schema), `internal/librarian/memsvc.go` (`SearchWiki`, `WikiSearchHit`), `schema.d/wiki.yaml` (designed but unimplemented fields) |
| **Other repos** | none consulted |

- Claims about **this** repository must be checked only against the **prd2wiki** row above.
- Claims about **pippi-librarian** (e.g. Part 1 “Verified against pippi-librarian source”) must name that repo’s path and commit; do not assume it matches another reviewer’s checkout.
- Design-only docs under `docs/wiki-local/` are **not** implementation; cite Go code in the scoped tree when asserting behavior.

---

## Part 1: How the System Is Supposed to Work

### Intended architecture

```
User edits page → wiki writes to git → wiki calls librarian via unix socket
    → librarian embeds via TEI → librarian stores in LanceDB
    → librarian indexes: BM25 + vector → RRF fusion (k=60)

User searches → wiki calls librarian memory_search via unix socket
    → librarian runs BM25 + vector + RRF → returns matches with page_uuid, score, title, snippet
    → wiki renders results
```

**Verified against pippi-librarian source** (`/srv/pippi-librarian`, branch `main`, commit `cc96c4b`):
- `internal/librarian/table.go` function `SearchWithFilter`: `fused := rrfFuse(lexRanked, vecRanked, 60)` — BM25 + vector + RRF k=60 confirmed
- `internal/librarian/memsvc.go` type `WikiSearchHit`: returns `PageUUID`, `RecordID`, `Title`, `Snippet`, `Score`, `HistoryCount`
- `cmd/pippi-librarian/main.go` memory_search handler: serializes matches array with all WikiSearchHit fields

**One embedder (TEI).** Called by the librarian only. The wiki never embeds.
**One vector database (LanceDB).** Inside the librarian. The wiki never stores vectors.
**One search path.** Wiki → librarian → LanceDB → results.
**SQLite FTS** in the wiki is for page listing/filtering and serves as degraded fallback.

---

## Part 2: How the System Actually Works Today

### Complete inventory (verified against source, function name anchors)

| # | What | Function/Location | What it does | Action |
|---|------|-------------------|-------------|--------|
| 1 | Wiki embedder creation | `app.go` `New()`: `embedder.NewOpenAIEmbedder(embCfg)` | Connects to TEI independently | **REMOVE** |
| 2 | JSON vector store creation | `app.go` `New()`: `vectordb.NewStore(emb)` | Creates in-memory vector store | **REMOVE** |
| 3 | Load vectors from disk | `app.go` `New()`: `vstore.LoadFromDisk(vectorPath)` | Loads `data/vectors/pages.json` | **REMOVE** |
| 4 | Auto-save persist path | `app.go` `New()`: `vstore.SetPersistPath(vectorPath)` | Saves JSON on every IndexPage | **REMOVE** |
| 5 | Vector rebuild goroutine | `app.go` `New()`: `if vstore.Count() == 0` block | Re-embeds ALL pages via TEI | **REMOVE** |
| 6 | Embedding profile store | `app.go` `New()`: `embedder.NewEmbeddingProfileStore(db)` | Tracks model versions | **REMOVE** |
| 7 | `VStore` on App struct | `app.go` struct `App`: `VStore *vectordb.Store` | Exposes store — **no external readers** (grep verified) | **REMOVE** |
| 8 | `vstore` on Librarian struct | `librarian.go` struct `Librarian`: `vstore` field | Local vector store reference | **REMOVE** |
| 9 | `Librarian.Search()` → local store | `librarian.go` `Search()`: `l.vstore.Search()` | In-memory fused search: 0.7×cosine + 0.3×keyword (NOT pure cosine) | **REWRITE** → call librarian |
| 10 | `Librarian.FindSimilar()` → local store | `librarian.go` `FindSimilar()`: `l.vstore.FindSimilar()` | Find similar pages locally | **REWRITE** → call librarian MemorySearch with page content |
| 11 | `Librarian.RemoveFromIndexes()` → local store | `librarian.go` `RemoveFromIndexes()`: `l.vstore.RemovePage()` + `l.indexer.RemovePage()` | Called from `tree_api.go` delete handler | **FIX** — keep SQLite `indexer.RemovePage()`, replace vstore call with async `memory_delete` via libclient (or deletes orphan librarian records) |
| 12 | `indexInVectorStore()` | `librarian.go` `indexInVectorStore()` | Chunks page, **prepends title+tags+type**, embeds, stores in JSON | **REMOVE** — but see Part 5 on content prefixing |
| 13 | `RebuildVectorIndex()` | `librarian.go` `RebuildVectorIndex()` | Iterates all pages, embeds each | **REMOVE** |
| 14 | Async local embed on write | `librarian.go` `submit()`: goroutine calling `indexInVectorStore` | Embeds in background after git | **REMOVE** — librarian does this via syncToLibrarian |
| 15 | **`DedupDetector`** | `dedup.go` `Check()`: `d.store.Search()` | 59 lines, 0.85 threshold — **BUT NEVER CALLED** | **REMOVE** (dead code) |
| 16 | **`flags.dedup`** | `librarian.go` `submitFlagsForIntent()` | Set true for integrate intent — **BUT NEVER READ** in `submit()` | **REMOVE** (dead code) |
| 17 | **API search: parallel FTS+vector merge** | `api/search.go` `searchPages()` | Runs FTS and `lib.Search()` in parallel goroutines, merges results (FTS first, then vector hits not in FTS) | **REWRITE** — lib.Search() must go through librarian |
| 18 | Web search: vector-first, FTS only if empty | `web/search.go` `searchPages()` | Calls `lib.Search()` (local vector); **only if `len(items)==0`** falls back to FTS. **NOT the same merge behavior as API.** | **REWRITE** — lib.Search() through librarian; **NOTE: web and API have different search semantics (see Part 5b)** |
| 20 | `normalizer.go` imports `vectordb.TextChunk` | `librarian/normalizer.go` `ChunkByHeadings()` | Returns `[]vectordb.TextChunk` — **blocks deletion** of `internal/vectordb/` | **MOVE** `TextChunk` type to `librarian` package or a neutral package before deleting `vectordb` |
| 19 | Embedder config in prd2wiki.yaml | `config/prd2wiki.yaml` `embedder:` section | TEI endpoint, dims, timeout | **REMOVE** |
| 21 | Stale comment in `app.go` | `app.go` `New()`: comment says "LlamaCpp" but code uses `NewOpenAIEmbedder` | Misleading — stale from rename | **FIX** comment during embedder removal |

### What uses the librarian (complete)

| What | Function | Status |
|------|----------|--------|
| `maybeSyncToLibrarian()` | Called at end of `submit()`, launches goroutine | **Works** |
| `runSyncToLibrarian()` | Async goroutine, calls `libClient.MemoryStore()` | **Works** — sends raw body (NOT title-prefixed, see Part 5) |
| `libclient.MemoryStore()` | Calls memory_store over socket | **Works** |
| `libclient.MemorySearch()` | Does not exist | **MISSING** — must be built |

### Dead code (verification review correction)

**DedupDetector is dead.** `flags.dedup` is set to `true` for the `integrate` intent by `submitFlagsForIntent()`, but `submit()` never reads `flags.dedup`. The dedup block was accidentally removed during the BUG-012/014 async embedding fix. The `DedupDetector` type, its tests, and the `dedup.go` file exist but are never invoked in production. This is not a feature to preserve — it's dead code to remove.

### libclient startup behavior (verification review correction)

`libclient.New()` returns `(*Client, error)`. On dial failure, it returns a **non-nil client** plus an error. `app.go` logs the error but does NOT clear `pippi`. So `WithPippiLibrarian` is wired even when the socket was unreachable. The "sync enabled" log means "client struct exists" — NOT "socket is healthy." Every `MemoryStore` / `MemorySearch` call will fail at request time until the socket actually works. This is misleading for operators and should be fixed in this effort.

---

## Part 3: How We Got Here

1. prd2wiki built first with own embedder + JSON vector store. Correct at the time.
2. Librarian designed to replace this. All docs say so. 13 audit rounds.
3. Audits reviewed documents, not code.
4. Phase 3 added librarian sync alongside old code. Nobody removed the old code.
5. BUG-012/014 fix accidentally removed the dedup call, making it dead code.
6. I reviewed and merged without catching the parallel pipeline.

### Anti-patterns (steward §6)

- §6.1 Confident Architect: docs described intended system, nobody verified code matched
- §6.2 Premature Builder: implementing agent added new code alongside old
- §6.5 Performative Compliance: 13 audits, zero code checks
- §6.8 Clean vs Complete: tests pass, old pipeline still active

---

## Part 4: Constraint Declaration (steward §3.1)

### Hard constraints

1. SQLite FTS must continue to work — page listing, filtering, path resolution depend on it
2. `syncToLibrarian` must not be disrupted — only path to librarian
3. Page deletion via tree API must still remove from SQLite index
4. Wiki must start and serve without librarian running (FTS-only degraded mode)
5. Existing tests for non-vectordb functionality must pass

### Known anti-patterns

1. Adding alongside instead of replacing — this plan must REMOVE
2. Silent fallback — FTS fallback must LOG when it activates. **Note: neither `api/search.go` nor `web/search.go` have any `slog` calls today. Logging is NEW WORK, not "already satisfied."**
3. "Sync enabled" log when socket is dead — misleading, must be fixed

### Test invariants (files that reference vectordb and must be updated)

- `internal/web/web_test.go` — 2 functions create `vectordb.Store`
- `internal/librarian/librarian_test.go` — 1 function
- `internal/api/pages_test.go` — 1 function
- `internal/librarian/dedup_test.go` — 2 functions (delete with dead code)
- `internal/vectordb/store_test.go` — entire file (delete with package)

### Performance contracts

| Metric | Current | Required after |
|--------|---------|---------------|
| Startup time | 10-120s (blocked by vector rebuild) | <10s |
| Search (vector) | ~100ms local scan | Depends on librarian (~50-200ms over socket) |
| Search (FTS fallback) | ~10ms | Unchanged |
| HTTP write response | <2s (git + SQLite; local embed is async goroutine) | <2s (git + SQLite; librarian sync is async goroutine) |
| Background index time after write | 60-120s (local embed via TEI) | N/A — librarian handles async |

---

## Part 5: Critical Decision — Content Prefixing

### The problem

`indexInVectorStore()` prepends title + tags + type to each chunk before embedding:
```
{title} {tag1} {tag2} ... {type}

{chunk body text}
```

This is a **relevance contract** — without it, vector search matches on body content only and misses pages where the title/tags are the best match.

`syncToLibrarian` sends `string(req.Body)` — the **raw page body** WITHOUT this prefix. So the librarian currently embeds different content than the local store did.

### Options

**A: Send prefixed content to the librarian.** Modify `runSyncToLibrarian()` to prepend the same title+tags+type prefix to the content before sending. The librarian stores and embeds exactly what it receives.

**B: Let the librarian handle prefixing.** The metadata (title, type, status, tags) is already sent in the `metadata` map via `ext`. The librarian could prepend these to the content before embedding. This keeps the prefixing logic in one place (librarian) rather than the wiki.

**C: Rely on BM25.** The librarian's hybrid search includes BM25 lexical matching. Title and tag matches would come through BM25 even without vector prefixing. The vector component would match on body content only.

**Resolution (post Review 4 chat discovery):** Option B is the only correct answer, but it requires **pippi-librarian changes first**.

**The librarian drops metadata.** Verified against `/srv/pippi-librarian` commit `cc96c4b`:
- MCP handler for `memory_store` accepts `metadata` in the schema but **never reads `args["metadata"]`**
- `StoreWiki()` takes `(ctx, namespace, pageUUID, content)` — no metadata parameter
- `MemoryRecord` has no title, tags, type, or status fields
- LanceDB Arrow schema has no `ext_json`, `page_title`, `page_tags`, or `page_type` columns
- The `schema.d/wiki.yaml` extension fields were designed in Phase 1 but **never implemented in runtime code**

**This is a BLOCKER.** The wiki sends metadata. The schema accepts it. The handler drops it. The record doesn't store it. The search can't use it. See Part 10 for prerequisites.

**[DECISION: Option B — librarian handles metadata. But librarian must be fixed first (Part 10).]**

---

## Part 5b: Web vs API Search Behavior (deep review correction)

The web UI and API have **different** search behavior. This is NOT "the same merge logic":

| Surface | Current behavior | After migration |
|---------|-----------------|----------------|
| **API** (`api/search.go`) | Runs FTS and `lib.Search()` **in parallel**, merges (FTS rows first, then vector hits not in FTS) | Same merge — `lib.Search()` now calls librarian instead of local store |
| **Web** (`web/search.go`) | Calls `lib.Search()` first; **only if zero results** falls back to FTS | Same pattern — `lib.Search()` now calls librarian; FTS fallback only if librarian returns nothing |

**Implication:** API always shows FTS+vector merged results. Web only shows vector OR FTS, never both. This is an existing behavior difference, not introduced by this migration. The plan does not change this — it just routes `lib.Search()` through the librarian instead of the local store.

**[REVIEWER: Decide if this UX difference should be aligned (both merge) or preserved (web = vector-first, API = merged). Not a blocker for this plan but should be documented.]**

### Search ranking will change

The local `vectordb.Store.Search` uses `0.7*cosine + 0.3*keywordScore` fusion. The librarian uses BM25 + vector + RRF (k=60). These are **different ranking algorithms**. Search results will be ordered differently after migration. This is **intended** — the librarian's ranking is better — but implementers should not claim "identical behavior."

### Vocabulary normalization

`Librarian.Search()` currently normalizes query tokens via `l.vocab.Normalize()` before searching. Whether the librarian's `memory_search` applies equivalent normalization is **unknown from this repo alone** (pippi-librarian is a separate codebase). Flag as a **parity assumption** — if search quality degrades for normalized terms, this is the place to look.

---

## Part 6: Bulk Backfill Strategy

### The problem

Removing the local vector store means search depends entirely on the librarian having pages indexed. Pages get indexed via `syncToLibrarian` on edit — but pages that have NEVER been edited since migration won't be in the librarian.

### Solution

After removing the local vector store, run a bulk backfill that sends every page to the librarian:

```bash
# For each project, for each page, call memory_store via the librarian socket.
# NOTE: prd2wiki-import does NOT exist in this repo (design-only in wiki docs).
# Implement a one-off backfill script or binary that:
#   1. Walks each project's git repo for all pages
#   2. Reads frontmatter (title, type, status, tags) + body
#   3. Calls libclient.MemoryStore() for each page
#   4. Reports progress and failures
```

This is NOT optional. Without it, only recently-edited pages appear in vector search.

### When to run

After Step 15 (all code changes done), before declaring the work complete.

---

## Part 7: Implementation Steps

### Phase 1: Build new path (librarian search), test alongside old

**Step 1:** Add `MemorySearch()` to `internal/libclient/client.go`
- Calls librarian's `memory_search` MCP tool over socket
- Returns matches with: page_uuid, record_id, title, snippet, score, history_count
- Handles connection errors (returns error, caller decides fallback)

**Step 2:** Rewrite `Librarian.Search()` in `librarian.go`
- When `libClient != nil`: call `libClient.MemorySearch()`
- **Namespace mapping (Review 4 blocker):** `Librarian.Search()` receives `project` as a repo key (e.g., `"default"`). The librarian's `memory_search` expects `namespace` as `"wiki:{project-uuid}"`. The search call must resolve repo key → tree `Project.UUID` → `"wiki:" + uuid`. Use `treeHolder.Get().ProjectByRepoKey(project)` to get the UUID. Without this mapping, search hits the wrong namespace or returns empty.
- **Vocabulary normalization (Review 4 + Review 5):** Current `Search()` normalizes query tokens via `l.vocab.Normalize()`. The librarian uses `NormalizeSemantic(query)` internally — these are DIFFERENT normalization functions. Decision: **send the wiki-normalized query to the librarian.** The librarian will apply its own normalization on top. This is double normalization with different rules — functionally harmless (both produce cleaner text) but NOT equivalent to either alone. If search quality changes for specific terms, this is the place to investigate.
- When `libClient == nil` or call fails: **return error** (not FTS fallback — `Librarian` does not have an `index.Searcher`)
- FTS fallback happens in the **handlers** (`api/search.go` and `web/search.go`), NOT in `Librarian.Search()`
- API handler: already runs FTS in parallel — if `lib.Search()` errors, the vector leg is empty but FTS results still display
- Web handler: if `lib.Search()` errors, `items` stays empty, existing FTS fallback runs
- Both handlers must `slog.Warn("librarian search unavailable, FTS only")` when `lib.Search()` fails (new logging — does not exist today)

**Step 3:** Rewrite `Librarian.FindSimilar()` in `librarian.go`
- Call `libClient.MemorySearch()` with the page's content as query
- Fallback: return empty results with log

**Step 4:** ~~Fix content prefixing in `runSyncToLibrarian()`~~ **REMOVED — contradicts Part 5 Option B**
- Part 5 decision: librarian handles prefixing (Option B) after Part 10 ships
- `runSyncToLibrarian` already sends metadata (title, tags, type, status) in the `ext` map
- The librarian will use this metadata to enrich embeddings (Part 10.4)
- The wiki does NOT prepend — no double enrichment

**Step 5:** Fix `libclient.New()` startup logging
- When dial fails: log ERROR and set `pippi = nil` so "sync enabled" only appears when socket is actually reachable
- Or: keep client but add a `Connected() bool` method and log accurately

**Step 6:** Test Phase 1
- With librarian running: search returns librarian results
- With librarian down: search returns FTS results with warning
- `go build` and `go test` pass

**Gate: Phase 1 passes before proceeding.**

### Phase 2: Remove old pipeline

**Step 7:** Remove `indexInVectorStore()` and its async call from `submit()`

**Step 8:** Remove dead dedup code: `flags.dedup` field, `DedupDetector` usage, `dedup.go`, `dedup_test.go`

**Step 9:** Remove `RebuildVectorIndex()` and its call from `app.go`

**Step 10:** Remove `vstore` from Librarian struct and `New()` signature. Update all callers.

**Step 11:** Fix `RemoveFromIndexes()`:
- Keep `indexer.RemovePage()` (SQLite — legitimate)
- Replace `vstore.RemovePage()` with async librarian delete (if libclient connected)
- **Review 5 BLOCKER:** `memory_delete` MCP tool takes `id` (mem_ record ID), NOT `page_uuid`. But `RemoveFromIndexes` is called with `page_uuid` from `tree_api.go`. Resolution options:
  - **A:** Read `.link` line 2 to get the current `mem_` head ID, pass that to `memory_delete`
  - **B:** Call `memory_get` by `page_uuid` first to get `record_id`, then `memory_delete` by `record_id`
  - **C:** Add a new librarian operation `DeleteByPageUUID` that handles the lookup internally
- **Recommendation:** Option A (read .link line 2) — simplest, no extra RPC, data is already on disk
- If libclient is nil (librarian down), log warning — orphan cleaned by compactor

**Step 11b:** Add `MemoryDelete()` to `internal/libclient/client.go`
- Calls librarian's `memory_delete` MCP tool over socket
- Takes `id` (mem_ record ID, NOT page_uuid) — matches the librarian's API

**Step 12:** Remove embedder + vector store creation from `app.go`: `NewOpenAIEmbedder()`, `vectordb.NewStore()`, `LoadFromDisk()`, `SetPersistPath()`, embedding profile store, `VStore` from App struct

**Step 13:** Remove `embedder:` section from `config/prd2wiki.yaml`

**Step 14:** Update `api/search.go` `searchPages()`: the parallel FTS+vector merge now calls the rewritten `lib.Search()` which goes through the librarian. The merge logic stays the same — FTS results first, librarian results merged in.

**Step 15:** Update `web/search.go` `searchPages()`: same — `lib.Search()` now goes through librarian.

**Step 16:** Update tests: `web_test.go`, `librarian_test.go`, `pages_test.go` — pass nil for vstore or update `New()` call. Delete `vectordb/store_test.go`.

**Step 16b:** Move `vectordb.TextChunk` type to `internal/librarian/` package
- `normalizer.go` `ChunkByHeadings()` returns `[]vectordb.TextChunk`
- Cannot delete `internal/vectordb/` until this type is moved
- Move the struct definition, update `normalizer.go` import

**Step 17:** Delete `internal/vectordb/` package (now safe — `TextChunk` moved in Step 16b)

**Step 18:** Delete `data/vectors/` directory and `pages.json`

### Phase 3: Backfill + Verify

**Step 19:** Run bulk backfill — send all pages to librarian

**Step 20:** End-to-end verification:
- Edit a page, search for it — results come from librarian
- Check librarian logs for `memory_search` calls
- Confirm wiki does NOT connect to TEI (no embedder log)
- Confirm wiki starts in <10 seconds
- Confirm `data/vectors/` not recreated
- `go build ./...` and `go test ./...` pass

---

## Part 10: Librarian Prerequisites (pippi-librarian changes, BLOCKER)

The following changes must be made in `/srv/pippi-librarian/` BEFORE this plan can be implemented. Without them, removing the local vector store degrades search quality because the librarian cannot match on title, tags, or type.

**Verified against `/srv/pippi-librarian` commit `cc96c4b`:**

### 10.1: MCP handler must pass metadata to StoreWiki

`cmd/pippi-librarian/main.go` memory_store handler currently calls:
```go
memStore.StoreWiki(ctx, namespace, pageUUID, content)
```
Must become:
```go
memStore.StoreWiki(ctx, namespace, pageUUID, content, metadata)
```
Where `metadata = args["metadata"]` (the map the wiki already sends).

### 10.2: StoreWiki must accept and store metadata

`internal/librarian/memory.go` `StoreWiki()` must:
- Accept `metadata map[string]any` parameter
- Store metadata fields on `MemoryRecord` (either as individual fields or `ExtJSON map[string]any`)
- At minimum: `page_title`, `page_type`, `page_status`, `page_tags`, `author`

### 10.3: LanceDB Arrow schema must include metadata columns

`internal/librarian/memory_lance.go` must add to the Arrow schema:
- `page_title` (string) — for BM25 + embedding enrichment
- `page_tags` (string) — for BM25 + embedding enrichment + filtering
- `page_type` (string) — for filtering
- `page_status` (string) — for filtering
- `ext_json` (string) — for additional metadata (author, source_repo, source_commit)

### 10.3b: Search title derivation (nuance from Review 5)

`SearchWiki` currently sets result `Title` from `firstLineTitle(rec.Content)` — parsing the first line of the body text. This may differ from the wiki's frontmatter title. After 10.2 ships, `SearchWiki` should use the stored `page_title` metadata instead of `firstLineTitle`.

### 10.4: Embedding must include title and tags

When the librarian embeds content for vector search, it must prepend title and tags to the content text. This ensures vector similarity matches on page identity, not just body content. The metadata for this is available from 10.2.

### 10.5: Search must support metadata filtering

`SearchWiki` should support filtering by type, status, tags when the caller requests it. This enables the wiki's structured search (filter by type, filter by status) to work through the librarian.

### 10.6: Bulk backfill must include metadata

Step 19 (bulk backfill) must send metadata WITH each page. Without it, records in the librarian will have title/tags = empty even after re-ingestion.

### Gate for Part 10

- [ ] Librarian `memory_store` stores title, tags, type, status from metadata
- [ ] LanceDB records include metadata columns
- [ ] Embedding text includes title + tags
- [ ] `memory_search` returns results that match on title (test: search "pippi readme" returns the README page in top 3)

**This gate must pass before proceeding with the prd2wiki plan.**

---

## Part 8: Rollback Plan

If partially done and search is broken:
1. `git revert` the branch
2. Restart wiki — old vector store code rebuilds from persisted JSON

If JSON file was deleted:
1. Revert code, restart — wiki re-embeds all pages (~5 min estimate, varies with corpus size and TEI speed)

---

## Part 9: Reviewer Verification Checklist

**Each must be checked against source code:**

1. **Is the inventory complete?** Run both greps — every result must appear in a step:
   - `grep -rn "vstore\|vectordb\|FindSimilar\|RemovePage\|IndexPage\|EmbedBatch" internal/ --include="*.go" | grep -v _test | grep -v ".bak"`
   - `grep -rn "TextChunk\|PageEmbedding" internal/ --include="*.go" | grep -v _test | grep -v ".bak"`

2. **Does syncToLibrarian send everything the librarian needs?** Verify: namespace, page_uuid, content (with title prefix per Part 5), title, type, status, tags.

3. **Does memory_search return enough for the wiki?** Verify: page_uuid, score, title, snippet at minimum.

4. **Are all pages bulk-imported into the librarian?** If not, vector search returns nothing. Step 19 is a BLOCKER, not optional.

5. **Does FindSimilar have any user-facing callers?** Verify no web handler or API route calls it. If it does, Step 3 must handle them.

6. **Does the API search merge behavior survive?** `api/search.go` runs FTS + vector in parallel. After this change, the "vector" leg calls the librarian. Verify the merge logic still works when results come from the librarian instead of local JSON.

7. **Does the content prefixing decision (Part 5) affect search quality?** Test: search for a page by title. Does the librarian find it? If not, the prefixing is a BLOCKER.

8. **Are there embedder config readers outside the vector store?** Run: `grep -rn "embedder\|Embedder" cmd/ config/ --include="*.go" --include="*.yaml"` — if the MCP server reads it, removing the config breaks it.

---

## Gate

- [ ] `internal/vectordb/` package deleted
- [ ] `data/vectors/` not created on startup
- [ ] Wiki does NOT connect to TEI
- [ ] Wiki does NOT embed pages locally
- [ ] Wiki does NOT rebuild vectors on startup
- [ ] `Librarian.Search()` calls librarian via libclient
- [ ] `Librarian.FindSimilar()` calls librarian via libclient
- [ ] Search falls back to SQLite FTS when librarian is down (with warning log)
- [ ] API search parallel merge works with librarian results
- [ ] Dead dedup code removed
- [ ] `RemoveFromIndexes()` keeps SQLite, replaces vstore with async `memory_delete`
- [ ] `MemoryDelete()` added to libclient
- [ ] `TextChunk` type moved to librarian package before vectordb deletion
- [ ] libclient startup logging is accurate (not "enabled" when socket is dead)
- [ ] Namespace mapping: `Librarian.Search()` resolves repo key → tree project UUID → `wiki:{uuid}` for `MemorySearch` call
- [ ] Query vocabulary normalization applied before `MemorySearch` call
- [ ] Bulk backfill completed — all pages in librarian WITH metadata
- [ ] Wiki starts in <10 seconds
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] End-to-end: edit → search via librarian → results render
- [ ] Librarian logs confirm memory_search calls from wiki

### Librarian prerequisite gate (Part 10 — must pass FIRST)

- [ ] Librarian `memory_store` handler reads and passes metadata to `StoreWiki`
- [ ] `StoreWiki` stores title, tags, type, status on the record
- [ ] LanceDB Arrow schema includes metadata columns
- [ ] Embedding text includes title + tags (search "pippi readme" → README in top 3)
- [ ] Reviewer has verified Part 10 claims against `/srv/pippi-librarian` at commit `cc96c4b` or later

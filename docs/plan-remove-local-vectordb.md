# PLAN: Remove Wiki's Local Vector Store — Use Librarian for All Search

**Date:** 2026-04-12 (revised after code verification review)
**Status:** DRAFT — REQUIRES REVIEW BEFORE IMPLEMENTATION
**Priority:** Critical
**Author:** Claude Opus 4.6
**Verification review:** `plan-remove-local-vectordb-RESPONSE-verification.md` — corrections incorporated below

**Reviewer instructions:** This plan must be examined for anything missing that would prevent the intended outcome. If a step, dependency, or integration point is missing, flag it as a BLOCKER. Every claim has been verified against source code as of 2026-04-12. Function names are used as anchors (not line numbers, which drift).

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

**Verified against pippi-librarian source:**
- `table.go` function `SearchWithFilter`: `fused := rrfFuse(lexRanked, vecRanked, 60)` — BM25 + vector + RRF k=60 confirmed
- `memsvc.go` type `WikiSearchHit`: returns `PageUUID`, `RecordID`, `Title`, `Snippet`, `Score`, `HistoryCount`
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
| 1 | Wiki embedder creation | `app.go` `Run()`: `embedder.NewOpenAIEmbedder(embCfg)` | Connects to TEI independently | **REMOVE** |
| 2 | JSON vector store creation | `app.go` `Run()`: `vectordb.NewStore(emb)` | Creates in-memory vector store | **REMOVE** |
| 3 | Load vectors from disk | `app.go` `Run()`: `vstore.LoadFromDisk(vectorPath)` | Loads `data/vectors/pages.json` | **REMOVE** |
| 4 | Auto-save persist path | `app.go` `Run()`: `vstore.SetPersistPath(vectorPath)` | Saves JSON on every IndexPage | **REMOVE** |
| 5 | Vector rebuild goroutine | `app.go` `Run()`: `if vstore.Count() == 0` block | Re-embeds ALL pages via TEI | **REMOVE** |
| 6 | Embedding profile store | `app.go` `Run()`: `embedder.NewEmbeddingProfileStore(db)` | Tracks model versions | **REMOVE** |
| 7 | `VStore` on App struct | `app.go` struct `App`: `VStore *vectordb.Store` | Exposes store — **no external readers** (grep verified) | **REMOVE** |
| 8 | `vstore` on Librarian struct | `librarian.go` struct `Librarian`: `vstore` field | Local vector store reference | **REMOVE** |
| 9 | `Librarian.Search()` → local store | `librarian.go` `Search()`: `l.vstore.Search()` | JSON array cosine scan | **REWRITE** → call librarian |
| 10 | `Librarian.FindSimilar()` → local store | `librarian.go` `FindSimilar()`: `l.vstore.FindSimilar()` | Find similar pages locally | **REWRITE** → call librarian MemorySearch with page content |
| 11 | `Librarian.RemoveFromIndexes()` → local store | `librarian.go` `RemoveFromIndexes()`: `l.vstore.RemovePage()` + `l.indexer.RemovePage()` | Called from `tree_api.go` delete handler | **FIX** — keep SQLite `indexer.RemovePage()`, remove vstore call |
| 12 | `indexInVectorStore()` | `librarian.go` `indexInVectorStore()` | Chunks page, **prepends title+tags+type**, embeds, stores in JSON | **REMOVE** — but see Part 5 on content prefixing |
| 13 | `RebuildVectorIndex()` | `librarian.go` `RebuildVectorIndex()` | Iterates all pages, embeds each | **REMOVE** |
| 14 | Async local embed on write | `librarian.go` `submit()`: goroutine calling `indexInVectorStore` | Embeds in background after git | **REMOVE** — librarian does this via syncToLibrarian |
| 15 | **`DedupDetector`** | `dedup.go` `Check()`: `d.store.Search()` | 59 lines, 0.85 threshold — **BUT NEVER CALLED** | **REMOVE** (dead code) |
| 16 | **`flags.dedup`** | `librarian.go` `submitFlagsForIntent()` | Set true for integrate intent — **BUT NEVER READ** in `submit()` | **REMOVE** (dead code) |
| 17 | **API search: parallel FTS+vector merge** | `api/search.go` `searchPages()` | Runs FTS and `lib.Search()` in parallel goroutines, merges results (FTS first, then vector hits not in FTS) | **REWRITE** — lib.Search() must go through librarian |
| 18 | Web search: vector then FTS fallback | `web/search.go` `searchPages()` | Calls `lib.Search()` (local vector), falls back to FTS | **REWRITE** — same as #17 |
| 19 | Embedder config in prd2wiki.yaml | `config/prd2wiki.yaml` `embedder:` section | TEI endpoint, dims, timeout | **REMOVE** |

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
2. Silent fallback — FTS fallback must LOG when it activates
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
| Write latency | 60-120s (local embed + git + sync) | <2s (git + async sync only) |

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

**Recommendation:** Option A (send prefixed content) for immediate parity. File a follow-up for Option B (move logic to librarian side) as the cleaner long-term solution.

**[REVIEWER: This affects search quality. Decide before implementation.]**

---

## Part 6: Bulk Backfill Strategy

### The problem

Removing the local vector store means search depends entirely on the librarian having pages indexed. Pages get indexed via `syncToLibrarian` on edit — but pages that have NEVER been edited since migration won't be in the librarian.

### Solution

After removing the local vector store, run a bulk backfill that sends every page to the librarian:

```bash
# For each project, for each page, call memory_store via the librarian socket
# This is what prd2wiki-import already does (Phase 5)
# Or: a simpler script that walks git and calls the MCP tool
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
- When `libClient == nil` or call fails: fall back to SQLite FTS with `slog.Warn("librarian search unavailable, using FTS fallback")`
- Do NOT call `l.vstore.Search()` anymore

**Step 3:** Rewrite `Librarian.FindSimilar()` in `librarian.go`
- Call `libClient.MemorySearch()` with the page's content as query
- Fallback: return empty results with log

**Step 4:** Fix content prefixing in `runSyncToLibrarian()`
- Prepend title + tags + type to content before sending to librarian (Part 5, Option A)
- This ensures the librarian embeds the same enriched content

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

**Step 11:** Fix `RemoveFromIndexes()`: keep `indexer.RemovePage()`, remove `vstore.RemovePage()`

**Step 12:** Remove embedder + vector store creation from `app.go`: `NewOpenAIEmbedder()`, `vectordb.NewStore()`, `LoadFromDisk()`, `SetPersistPath()`, embedding profile store, `VStore` from App struct

**Step 13:** Remove `embedder:` section from `config/prd2wiki.yaml`

**Step 14:** Update `api/search.go` `searchPages()`: the parallel FTS+vector merge now calls the rewritten `lib.Search()` which goes through the librarian. The merge logic stays the same — FTS results first, librarian results merged in.

**Step 15:** Update `web/search.go` `searchPages()`: same — `lib.Search()` now goes through librarian.

**Step 16:** Update tests: `web_test.go`, `librarian_test.go`, `pages_test.go` — pass nil for vstore or update `New()` call. Delete `vectordb/store_test.go`.

**Step 17:** Delete `internal/vectordb/` package

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

## Part 8: Rollback Plan

If partially done and search is broken:
1. `git revert` the branch
2. Restart wiki — old vector store code rebuilds from persisted JSON

If JSON file was deleted:
1. Revert code, restart — wiki re-embeds all pages (~5 min)

---

## Part 9: Reviewer Verification Checklist

**Each must be checked against source code:**

1. **Is the inventory complete?** Run: `grep -rn "vstore\|vectordb\|FindSimilar\|RemovePage\|IndexPage\|EmbedBatch" internal/ --include="*.go" | grep -v _test | grep -v ".bak"` — every result must appear in a step.

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
- [ ] `RemoveFromIndexes()` keeps SQLite, drops vstore
- [ ] Content sent to librarian includes title/tags prefix
- [ ] libclient startup logging is accurate (not "enabled" when socket is dead)
- [ ] Bulk backfill completed — all pages in librarian
- [ ] Wiki starts in <10 seconds
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] End-to-end: edit → search via librarian → results render
- [ ] Librarian logs confirm memory_search calls from wiki

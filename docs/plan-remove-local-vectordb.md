# PLAN: Remove Wiki's Local Vector Store — Use Librarian for All Search

**Date:** 2026-04-12
**Status:** DRAFT — REQUIRES REVIEW BEFORE IMPLEMENTATION
**Priority:** Critical
**Author:** Claude Opus 4.6 (this session)

**Reviewer instructions:** This plan must be examined for anything missing that would prevent the intended outcome. If a step, dependency, or integration point is missing, flag it as a BLOCKER. The pattern of "looks wired but isn't" has occurred repeatedly in this project. The reviewer MUST verify claims against actual code at the cited locations, not against design documents.

---

## Part 1: How the System Is Supposed to Work

### Intended architecture (verified against pippi-librarian source 2026-04-12)

```
User edits page → wiki writes to git → wiki calls librarian via unix socket
    → librarian embeds via TEI → librarian stores in LanceDB (pippi_memory)
    → librarian indexes: BM25 lexical + vector cosine → RRF fusion (k=60)

User searches → wiki calls librarian memory_search via unix socket
    → librarian runs BM25 + vector + RRF → returns matches
    → wiki renders results
```

**Verified against source:** `/srv/pippi-librarian/internal/librarian/table.go:299-300`:
```go
fused := rrfFuse(lexRanked, vecRanked, 60)
```
BM25 + vector + RRF k=60 is confirmed in current code, not just design docs.

**One embedder (TEI).** Called by the librarian only.
**One vector database (LanceDB).** Inside the librarian.
**One search path.** Wiki → librarian → LanceDB → results.
**SQLite FTS** in the wiki is for page listing/filtering (metadata). Serves as degraded fallback when librarian is down.

### What the librarian's memory_search returns

Verified against `/srv/pippi-librarian/cmd/pippi-librarian/main.go` and `/srv/pippi-librarian/internal/librarian/memsvc.go`:

```json
{
  "matches": [
    {
      "page_uuid": "550e8400-...",
      "record_id": "mem_0hf5e3a8...",
      "title": "Page Title",
      "snippet": "...matching text...",
      "score": 0.92,
      "history_count": 3
    }
  ]
}
```

This provides everything the wiki needs to render search results.

---

## Part 2: How the System Actually Works Today

### Complete inventory of local vector store usage

Verified by grep against current source 2026-04-12. Every code path listed was confirmed at the cited location.

| # | What | File:Line | What it does | Should exist? |
|---|------|-----------|-------------|---------------|
| 1 | Wiki embedder creation | `app.go:199-208` | Connects to TEI, creates OpenAIEmbedder | **NO** |
| 2 | JSON vector store creation | `app.go:209` | `vectordb.NewStore(emb)` | **NO** |
| 3 | Load vectors from disk | `app.go:212-219` | Loads `data/vectors/pages.json` | **NO** |
| 4 | Vector rebuild goroutine | `app.go:261-291` | Re-embeds ALL pages via TEI on startup | **NO** |
| 5 | Embedding profile store | `app.go:222-234` | Tracks embedding model versions | **NO** (librarian manages this) |
| 6 | `VStore` on App struct | `app.go:69` | Exposes vector store externally | **NO** — verified no external readers by grep |
| 7 | `vstore` on Librarian struct | `librarian.go:85` | Local vector store reference | **NO** |
| 8 | `lib.Search()` → local store | `librarian.go:230` | `l.vstore.Search()` → JSON array scan | **NO** — should call librarian |
| 9 | `lib.FindSimilar()` → local store | `librarian.go:247-248` | `l.vstore.FindSimilar()` → JSON scan | **NO** — feature needs replacement or removal |
| 10 | `RemoveFromIndexes()` → local store | `librarian.go:305-310` | `l.vstore.RemovePage()` + SQLite remove | **PARTIAL** — keep SQLite remove, drop vstore remove |
| 11 | `indexInVectorStore()` | `librarian.go:389-418` | Chunks page, embeds locally, stores in JSON | **NO** — librarian does this |
| 12 | `RebuildVectorIndex()` | `librarian.go:265-303` | Iterates all pages, embeds each locally | **NO** |
| 13 | Async local embed on write | `librarian.go:469-471` (goroutine at 481-486) | Embeds in background after git write | **NO** — librarian does this via syncToLibrarian |
| 14 | DedupDetector → local store | `dedup.go:34` | `d.store.Search()` — fully implemented (59 lines, 0.85 threshold) | **NEEDS DECISION** — wire to librarian or remove |
| 15 | Dedup call in submit() | `librarian.go:489-496` | Calls DedupDetector after write | **NEEDS DECISION** |
| 16 | `tree_api.go:365` → RemoveFromIndexes | `api/tree_api.go:365` | Called on page delete via tree API | **FIX** — keep SQLite part, remove vstore part |
| 17 | `api/pages.go:212` → indexer.RemovePage | `api/pages.go:212` | SQLite only — legitimate, keep | **KEEP** |

### What actually uses the librarian (complete list)

| What | File:Line | Status |
|------|-----------|--------|
| `maybeSyncToLibrarian()` | `librarian.go:165-181` | **Works** — called at end of submit(), launches goroutine |
| `runSyncToLibrarian()` | `librarian.go:184-212` | **Works** — async goroutine, calls `libClient.MemoryStore()`, writes .link line 2 |
| `libclient.MemoryStore()` | `libclient/client.go:72` | **Works** — calls memory_store over socket |
| `libclient.MemorySearch()` | Does not exist | **MISSING** — must be built |

### syncToLibrarian calling convention (corrected)

`maybeSyncToLibrarian()` is called synchronously at the end of `submit()` (line 497), but it launches a **goroutine** at line 181: `go l.runSyncToLibrarian(...)`. So: the decision to sync is synchronous, the actual sync is async. The wiki does not block on the librarian response.

### What syncToLibrarian sends to the librarian

Verified at `librarian.go:184-212`:
- `namespace`: `"wiki:" + projectUUID`
- `page_uuid`: from .link line 1 or frontmatter ID
- `content`: full page body
- `metadata`: source_repo, source_branch, source_commit, author, page_title, page_type, page_status, page_tags

This is everything the librarian needs for search (content for embedding, metadata for filtering).

---

## Part 3: How We Got Here

1. **prd2wiki built first** with its own embedder + JSON vector store. Correct at the time (no librarian).
2. **Librarian designed** to replace wiki's search. All design docs say this. 13 audit rounds reviewed docs.
3. **Audits reviewed documents, not code.** No audit verified the old vector store was removed.
4. **Phase 3 implementation** added librarian sync AS AN ADDITION. Nobody removed the old code.
5. **I reviewed and merged** without catching the parallel pipeline.

### Anti-patterns that occurred (per steward §6)

| Anti-pattern | How it manifested |
|-------------|-------------------|
| §6.1 Confident Architect | Design docs described the intended system. Nobody verified the actual system matched. |
| §6.2 Premature Builder | Implementing agent saw existing working code and left it, adding new code alongside. |
| §6.5 Performative Compliance | 13 audit rounds produced correct documents. Zero audits checked the code. |
| §6.8 Clean vs Complete | Tests pass, code committed. But the old pipeline was never removed — debt transferred. |

---

## Part 4: Constraint Declaration (per steward §3.1)

### Hard constraints

1. **SQLite FTS must continue to work** — page listing, filtering by type/status/tag, path resolution all depend on it
2. **syncToLibrarian must not be disrupted** — it's the only path that sends pages to the librarian
3. **Page deletion via tree API must still work** — `tree_api.go:365` calls `RemoveFromIndexes()`; the SQLite removal must be preserved
4. **Wiki must start and serve without the librarian running** — degraded mode (FTS only) is acceptable
5. **Existing tests for non-vectordb functionality must continue to pass**

### Known anti-patterns (things that failed)

1. **Adding alongside instead of replacing** — the librarian was added next to the JSON store. This plan must REMOVE, not add alongside.
2. **Silent fallback hiding problems** — the FTS fallback should LOG when it activates, not silently serve degraded results
3. **Line number references drift** — use function names as anchors, not line numbers

### Test invariants

Tests that currently use `vectordb.Store` and must be updated:
- `internal/web/web_test.go` (2 test functions create vectordb.Store)
- `internal/librarian/dedup_test.go` (2 test functions)
- `internal/librarian/librarian_test.go` (1 test function)
- `internal/api/pages_test.go` (1 test function)
- `internal/vectordb/store_test.go` (entire file — delete with package)

Each must be rewritten to either pass nil for vstore or use a mock librarian client.

### Performance contracts

| Metric | Current | Required after |
|--------|---------|---------------|
| Wiki startup time | 10-120s (blocked by vector rebuild) | <10s (no vector rebuild) |
| Search latency (vector) | ~100ms (local JSON scan) | depends on librarian (should be similar or better with LanceDB) |
| Search latency (FTS fallback) | ~10ms | unchanged |
| Write latency | 60-120s (local embed + git + sync) | <2s (git + async sync, no local embed) |

---

## Part 5: Feature Decisions Required

### FindSimilar (related pages)

`Librarian.FindSimilar()` provides a "find pages similar to this one" feature. It works by searching the local vector store with the page's content.

**Options:**
- **A: Wire to librarian** — add `libclient.MemorySearch()` call with the page's content as the query. The librarian's hybrid search would find similar pages. This is functionally equivalent.
- **B: Remove the feature** — drop FindSimilar entirely. No current UI calls it (verified: no web handler or template references it). Only the API `/api/projects/{project}/pages/{id}/references` handler does, and it uses a different function (`search.References()`).
- **C: Defer** — leave FindSimilar as a stub that returns empty results when vstore is nil. Fix later.

**Recommendation:** Option B (remove). No UI uses it. The references endpoint uses SQLite, not FindSimilar. If needed later, Option A is straightforward.

**[REVIEWER: Confirm no UI or user-facing feature depends on FindSimilar before approving Option B.]**

### DedupDetector

`DedupDetector` is a fully implemented (59 lines) duplicate detection system that searches the local vector store for pages with >85% similarity. It runs during the `integrate` intent in `submit()`.

**Options:**
- **A: Wire to librarian** — call `libclient.MemorySearch()` with the page content, check if any result has score > 0.85.
- **B: Remove** — drop dedup entirely. It was designed as a quality gate but no user-facing feature depends on its output (warnings are returned in the API response but not displayed in the UI).
- **C: Defer** — skip dedup when vstore is nil. Implement via librarian later.

**Recommendation:** Option C (defer). Dedup is useful but not critical. Skip when vstore is nil, add a TODO. Wire to librarian in a follow-up.

**[REVIEWER: Confirm dedup warnings are not displayed in the UI before approving Option C.]**

---

## Part 6: Implementation Steps

### Phase 1: Add + wire (build new path before removing old)

**Step 1:** Add `MemorySearch()` to `internal/libclient/client.go`
- Calls librarian's `memory_search` MCP tool
- Returns `[]SearchResult` with page_uuid, record_id, title, snippet, score, history_count
- Handles connection errors gracefully (returns error, caller decides fallback)

**Step 2:** Rewrite `Librarian.Search()` in `internal/librarian/librarian.go`
- When `libClient != nil`: call `libClient.MemorySearch()` via socket
- When `libClient == nil` or call fails: fall back to SQLite FTS with `slog.Warn("librarian search unavailable, using FTS fallback")`
- Remove local vstore search path

**Step 3:** Test Phase 1
- With librarian running: search returns librarian results
- With librarian down: search returns FTS results with warning log
- `go build` and `go test` pass (vstore still exists but Search() no longer uses it)

**Gate: Phase 1 must pass before proceeding to Phase 2.**

### Phase 2: Remove old pipeline

**Step 4:** Remove `FindSimilar()` from `Librarian` (or stub to return nil)

**Step 5:** Remove `indexInVectorStore()` and its call from `submit()`
- Remove the async goroutine at lines 481-486 that embeds locally
- `syncToLibrarian` handles embedding via the librarian

**Step 6:** Remove `RebuildVectorIndex()` and its call from `app.go`

**Step 7:** Remove `vstore` field from `Librarian` struct
- Update `New()` signature: remove `vstore *vectordb.Store` parameter
- Update all callers in `app.go`

**Step 8:** Remove `DedupDetector` vstore dependency
- When vstore is nil, skip dedup in `submit()` with `slog.Info("dedup skipped: local vector store removed, will use librarian in future")`

**Step 9:** Fix `RemoveFromIndexes()`
- Keep `l.indexer.RemovePage(pageID)` (SQLite — legitimate)
- Remove `l.vstore.RemovePage(pageID)` call
- Optionally: add async `libclient.MemoryDelete()` call to remove from librarian too

**Step 10:** Remove embedder + vector store creation from `app.go`
- Remove: `NewOpenAIEmbedder()`, `vectordb.NewStore()`, `LoadFromDisk()`, `SetPersistPath()`, vector rebuild goroutine, embedding profile store
- Remove: `VStore` field from App struct (confirmed no external readers)
- Remove: `embedder:` section from `config/prd2wiki.yaml`

**Step 11:** Update tests
- `web_test.go`: pass nil for vstore in Librarian.New() or mock
- `librarian_test.go`: same
- `api/pages_test.go`: same
- `dedup_test.go`: update to handle nil vstore (skip or mock)
- Delete `vectordb/store_test.go` with the package

**Step 12:** Delete `internal/vectordb/` package

**Step 13:** Delete `data/vectors/pages.json` (runtime data)

**Step 14:** Delete `data/vectors/` directory

### Phase 3: Verify

**Step 15:** Full end-to-end test
- Start librarian + wiki
- Edit a page
- Search for it — confirm result comes from librarian (check librarian logs for memory_search)
- Confirm wiki does NOT connect to TEI (no embedder log at startup)
- Confirm wiki starts in <10 seconds
- Confirm `data/vectors/` is not recreated
- `go build ./...` passes
- `go test ./...` passes

---

## Part 7: Rollback Plan

If the migration is partially done and search is broken:
1. `git stash` or `git revert` the branch
2. Restart wiki from the reverted code
3. The old vector store will rebuild from the persisted JSON file

If the JSON file was already deleted:
1. Revert code
2. Wiki will re-embed all pages on startup (takes ~5 minutes)

---

## Part 8: What the Reviewer Must Verify

**Each of these must be checked against actual source code, not this document:**

1. **Is the inventory in Part 2 complete?** `grep -rn "vstore\|vectordb\|FindSimilar\|RemovePage\|IndexPage\|EmbedBatch" internal/ --include="*.go" | grep -v _test | grep -v ".bak"` — every result must be addressed in a step.

2. **Does syncToLibrarian send everything the librarian needs?** Check `runSyncToLibrarian()` sends namespace, page_uuid, content, title, type, status, tags. If any field is missing, search quality degrades.

3. **Does the librarian's memory_search return enough for the wiki?** The wiki needs at minimum: page_uuid and score. Title and snippet are helpful. Verify the MCP response shape.

4. **Are all pages in the librarian?** After migration, pages must be bulk-imported into the librarian. If this hasn't happened, vector search returns nothing even with correct wiring.

5. **Does FindSimilar have any user-facing callers?** Verify no web handler, template, or MCP tool calls it. If it does, Option B (remove) is wrong.

6. **Does DedupDetector output appear in the UI?** Check if dedup warnings from `submit()` are shown to users. If they are, Option C (defer) needs a UI change.

7. **Do any tests assert vectordb behavior that must be preserved?** Check if test assertions verify search quality or vector content, not just "doesn't crash."

8. **Does anything outside `internal/` read the embedder config?** `grep -rn "embedder\|Embedder" cmd/ config/ --include="*.go" --include="*.yaml"` — if the MCP server or other tools read it, removing the config breaks them.

---

## Gate (ALL must be true before declaring done)

- [ ] `internal/vectordb/` package deleted
- [ ] `data/vectors/` directory not created on startup
- [ ] Wiki does NOT connect to TEI on startup
- [ ] Wiki does NOT embed pages locally on write
- [ ] Wiki does NOT rebuild vectors on startup
- [ ] `Librarian.Search()` calls librarian via libclient when connected
- [ ] Search falls back to SQLite FTS when librarian is down (with warning log)
- [ ] `FindSimilar()` removed or stubbed (per reviewer decision)
- [ ] `DedupDetector` handles nil vstore (skips with log)
- [ ] `RemoveFromIndexes()` keeps SQLite remove, drops vstore remove
- [ ] Wiki starts in <10 seconds
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] End-to-end: edit → search via librarian → results render
- [ ] Librarian logs confirm memory_search calls
- [ ] Wiki logs confirm NO embedder/TEI connection

# Feature: Hybrid Search + Service Dashboard

**Date:** 2026-04-12
**Status:** Design — not yet implemented
**Source:** Greg's feedback during bug fixing

## 1. Race FTS + Vector Search

**Current:** Web search runs vector search first. If it returns 0 results, falls back to SQLite FTS. Results are tagged `[vector]` or `[sql]` for debugging.

**Proposed:** Run both searches in parallel. Display FTS results immediately (they're fast — <10ms). When vector results arrive, merge them in and update rankings. Best of both worlds.

### Implementation sketch

```go
// In searchPages handler:
type searchResult struct {
    items  []PageListItem
    source string // "fts" or "vector"
}

ch := make(chan searchResult, 2)

// FTS in goroutine
go func() {
    results, _ := h.search.Search(project, query, typ, status, tag)
    ch <- searchResult{items: toItems(results), source: "fts"}
}()

// Vector in goroutine  
go func() {
    results, _ := lib.Search(ctx, project, query, 20)
    ch <- searchResult{items: toItems(results), source: "vector"}
}()

// For SSE/streaming: send FTS results as they arrive, then vector results
// For traditional HTTP: wait for both, merge, render once
```

### For the UI (streaming option)

Use Server-Sent Events or a two-phase render:
1. First response: FTS results with status `[SEARCHING - VECTOR PENDING]`
2. Second update (via SSE or JS fetch): merged results with `[COMPLETE]`

### Simpler version (no streaming)

Run both in parallel with `errgroup`, merge results (dedupe by page UUID, keep highest score), render once. Total time = max(FTS time, vector time) instead of FTS + vector.

## 2. Search Status Indicators

Show the user what's happening:

| State | Display |
|-------|---------|
| FTS returned, vector pending | `[SEARCHING - VECTOR RUNNING]` |
| Both returned | `[COMPLETE - 12 results from FTS + 8 from vector]` |
| Vector unavailable | `[FTS ONLY - embedder not available]` |
| Rebuilding | `[VECTOR INDEX REBUILDING - 45/211 pages]` |

## 3. Service Dashboard

Add to `/admin` or a new `/status` endpoint:

| Service | Status | Details |
|---------|--------|---------|
| **TEI Embedder** | alive/dead | Last health check, response time |
| **Vector Index** | ready/rebuilding/empty | Entry count, rebuild progress (N/total pages) |
| **SQLite Index** | ready/rebuilding | Page count, last rebuild time |
| **Librarian Socket** | connected/disconnected | Last successful sync, error message |
| **Git Repos** | open | Per-project: branch count, page count |

### API endpoint

```
GET /api/status
```

```json
{
  "embedder": {"alive": true, "latency_ms": 15},
  "vector_index": {"entries": 2882, "rebuilding": false},
  "sqlite_index": {"pages": 211},
  "librarian": {"connected": false, "error": "socket not reachable"},
  "projects": {
    "default": {"branches": 3, "pages": 186},
    "battletech": {"branches": 1, "pages": 11}
  }
}
```

### Implementation notes

- The vector rebuild progress requires the rebuild goroutine to report progress (currently just logs). Add an atomic counter that the status endpoint reads.
- TEI health check: ping `/health` endpoint with timeout.
- Librarian status: check if libclient is non-nil and last sync succeeded.

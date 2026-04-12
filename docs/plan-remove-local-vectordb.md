# PLAN: Remove Wiki's Local Vector Store — Use Librarian for All Search

**Date:** 2026-04-12
**Status:** Plan — review before implementation
**Priority:** Critical — the wiki runs a parallel undocumented vector database that duplicates and conflicts with the librarian

## The Problem

The wiki has TWO search backends running simultaneously:

1. **`internal/vectordb/` (JSON vector store)** — a custom in-memory vector store that embeds pages locally via TEI, stores vectors in `data/vectors/pages.json`, and does cosine similarity scan on search. This was built before the librarian existed. It is NOT documented in any design page, architecture diagram, or plan.

2. **pippi-librarian (LanceDB)** — the designed search backend with BM25 + vector + RRF fusion, version-aware records, proper indexing. This is what we spent 61 hours building. The wiki writes to it (via `syncToLibrarian`) but NEVER reads from it for search.

### What this causes

- **Double embedding:** Every page edit sends content to TEI twice — once from the wiki's own embedder, once from the librarian when `syncToLibrarian` fires.
- **TEI crashes:** The wiki's vector rebuild on startup sends hundreds of pages to TEI in rapid succession, causing OOM kills.
- **Wrong search results:** The JSON vector store has no BM25, no RRF fusion, no metadata in embeddings. Search quality is poor.
- **Startup blocked:** The wiki waits for the JSON vector index to load/rebuild before serving — can take minutes.
- **Unused librarian:** The librarian's LanceDB with proper search sits idle. Nobody queries it.

## What to Remove

### Files to delete
- `internal/vectordb/store.go` — the JSON vector store
- `internal/vectordb/record.go` — vector record types
- `internal/vectordb/store_test.go` — tests
- `data/vectors/pages.json` — persisted vector data (runtime)

### Code to remove from `internal/app/app.go`
- Wiki's own embedder creation (`NewOpenAIEmbedder`, health check, noop fallback)
- `vectordb.NewStore(emb)` creation
- `vstore.LoadFromDisk()` / `vstore.SetPersistPath()`
- Embedding profile store creation
- Vector rebuild goroutine (the entire `if vstore.Count() == 0` block)
- `VStore` field from `App` struct

### Code to remove from `internal/librarian/librarian.go`
- `vstore` field and all calls to `l.vstore.Search()`, `l.vstore.IndexPage()`, `l.vstore.RemovePage()`
- `indexInVectorStore()` function — the librarian handles embedding internally
- `RebuildVectorIndex()` function — no local vector index to rebuild
- `ChunkByHeadings()` usage for local embedding (keep the function for other uses)

### Code to remove from `internal/web/search.go`
- Vector search path in web search handler (replaced by librarian call)
- `[vector]`/`[sql]` tagging (replaced by librarian-backed results)

### Code to remove from `internal/api/search.go`
- `lib.Search()` calls that go to local vector store

## What to Add

### `internal/libclient/client.go`
- `MemorySearch(ctx, namespace, query, limit)` — calls librarian's `memory_search` MCP tool

### `internal/librarian/librarian.go` — `Search()` method
- Calls `libclient.MemorySearch()` when librarian is connected
- Falls back to SQLite FTS when librarian is not available (graceful degradation)

### `internal/web/search.go` — web search handler
- Uses the rewritten `lib.Search()` which goes through the librarian
- SQLite FTS fallback already exists

## What to Keep

- **SQLite FTS (`internal/index/`)** — legitimate for page listing, filtering by type/status/tag, path resolution. This is metadata, not search.
- **`internal/embedder/` package** — keep the code but don't create an embedder instance at wiki startup. The librarian manages its own embedder.
- **`syncToLibrarian`** — continues to write pages to the librarian on edit.

## Config Changes

Remove from `config/prd2wiki.yaml`:
```yaml
# REMOVE:
embedder:
  endpoint: "http://localhost:8088"
  dimensions: 768
  timeout: 60s
```

The wiki doesn't need an embedder config. The librarian has its own.

## Execution Order

1. Add `MemorySearch` to libclient
2. Rewrite `Librarian.Search()` to call libclient (with FTS fallback)
3. Remove `vstore` from `Librarian` struct + `New()` signature
4. Remove vector store creation, loading, rebuild from `app.go`
5. Remove wiki's own embedder creation from `app.go`
6. Remove embedder config from `prd2wiki.yaml`
7. Delete `internal/vectordb/` package
8. Delete `data/vectors/pages.json`
9. Update tests
10. Build, test, verify search works through librarian

## Dependencies

- **Librarian must be running** for search to return vector results. If librarian is down, search degrades to SQLite FTS only (titles, tags, metadata — no semantic search).
- **Librarian must have pages indexed** — either via `syncToLibrarian` on page edits, or via bulk import using `prd2wiki-import` / `prd2wiki-migrate`.

## Risks

- If the librarian is down, search returns FTS results only (no vector similarity). This is acceptable — FTS covers title/tag matches which are the most common search pattern.
- If the librarian has never received any pages (fresh install), search returns FTS only until pages are edited or bulk-imported.

## Gate

- `go build ./...` passes
- `go test ./...` passes
- `internal/vectordb/` package deleted
- `data/vectors/pages.json` not created on startup
- Wiki does NOT connect to TEI embedder on startup
- Wiki does NOT embed pages locally
- Search goes through librarian when connected
- Search falls back to SQLite FTS when librarian is down
- No startup delay from vector loading/rebuilding

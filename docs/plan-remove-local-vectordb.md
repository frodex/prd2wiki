# PLAN: Remove Wiki's Local Vector Store — Use Librarian for All Search

**Date:** 2026-04-12
**Status:** DRAFT — REQUIRES REVIEW BEFORE IMPLEMENTATION
**Priority:** Critical
**Reviewer instructions:** This plan must be examined for anything missing that would prevent the intended outcome. If a step, dependency, or integration point is missing, flag it as a BLOCKER before implementation begins. The pattern of "looks wired but isn't" has happened repeatedly — the reviewer should verify each claim against actual code, not documentation.

---

## Part 1: How the System Is SUPPOSED to Work

### The design (from de836ff, c6525ac, 8634f02, 6ccd407)

```
User edits page in wiki
    ↓
Wiki writes to git (source of truth)
    ↓
Wiki calls pippi-librarian via unix socket (syncToLibrarian)
    ↓
Librarian embeds content via TEI (nomic-embed-text-v1.5, 768 dims)
    ↓
Librarian stores in LanceDB (pippi_memory table)
    with: BM25 index, vector index, record_type, page_uuid, version chain
    ↓
User searches in wiki
    ↓
Wiki calls pippi-librarian memory_search via unix socket
    ↓
Librarian runs hybrid search: BM25 (lexical) + vector (cosine) → RRF fusion (k=60)
    ↓
Results returned with page_uuid, score, title, snippet, history_count
```

**One embedder (TEI).** Called by the librarian only. The wiki never embeds anything.

**One vector database (LanceDB).** Inside the librarian. The wiki never stores vectors.

**One search path.** Wiki → librarian → LanceDB → results. No local search.

**SQLite FTS** in the wiki is for page listing, filtering by type/status/tag, and path resolution. It is metadata, not search. It serves as a degraded fallback if the librarian is down.

### What the constraint file says

From `/srv/prd2wiki/docs/constraints-prd2wiki-pippi.md`:
- "The librarian is the only way to read or write data. No direct database access."
- "Wiki sends full content to memory_store — the librarian handles everything else internally."

From the steward rules (`/srv/PHAT-TOAD-with-Trails/steward/system.md`):
- §3.2: "If someone touching your work could break it by not knowing something, it is a constraint — not an 'implementation detail.'"

---

## Part 2: How the System Actually Works Today

### What exists in the code (verified against actual source, not docs)

```
User edits page in wiki
    ↓
Wiki writes to git ✓
    ↓
Wiki embeds content LOCALLY via its own TEI connection ← WRONG
    ↓
Wiki stores vectors in data/vectors/pages.json (in-memory JSON array) ← WRONG
    ↓
Wiki ALSO calls syncToLibrarian (async, fire-and-forget)
    ↓
Librarian embeds content AGAIN via the SAME TEI ← DOUBLE WORK
    ↓
Librarian stores in LanceDB ← NEVER READ BY WIKI
    ↓
User searches in wiki
    ↓
Wiki searches its LOCAL JSON vector store ← WRONG, POOR QUALITY
    ↓
If no results, falls back to SQLite FTS
    ↓
Librarian's LanceDB with BM25+vector+RRF sits completely unused
```

### Specific code locations

| What | Where | What it does | Should it exist? |
|------|-------|-------------|-----------------|
| Wiki's own embedder | `app.go:199-208` | Connects to TEI independently | **NO** |
| JSON vector store creation | `app.go:209` | `vectordb.NewStore(emb)` | **NO** |
| JSON vector load from disk | `app.go:212-219` | Loads `data/vectors/pages.json` | **NO** |
| Vector rebuild on startup | `app.go:261-291` | Re-embeds ALL pages via TEI | **NO** — causes TEI OOM |
| Local embedding on write | `librarian.go:469-471` | Embeds page content locally | **NO** — librarian does this |
| `indexInVectorStore()` | `librarian.go:389-418` | Chunks page, embeds, stores in JSON | **NO** |
| `RebuildVectorIndex()` | `librarian.go:265-303` | Iterates all pages, embeds each | **NO** |
| `lib.Search()` → local store | `librarian.go:230` | `l.vstore.Search()` → JSON array | **NO** — should call librarian |
| `DedupDetector` → local store | `dedup.go:34` | Uses JSON vector store for similarity | **NO** |
| `internal/vectordb/` package | entire package | In-memory JSON vector store | **DELETE** |
| `data/vectors/pages.json` | runtime data | Persisted vector array | **DELETE** |
| Wiki embedder config | `prd2wiki.yaml:14-17` | `embedder:` section | **REMOVE** |

### Versus what actually uses the librarian

| What | Where | Status |
|------|-------|--------|
| `syncToLibrarian` | `librarian.go:184` | **Works** — writes to librarian on edit |
| `libclient.MemoryStore()` | `libclient/client.go:72` | **Works** — calls memory_store |
| `libclient.MemorySearch()` | Does not exist | **MISSING** — never built |
| Search via librarian | Does not exist | **MISSING** — search calls local JSON store |

---

## Part 3: How We Got Here

1. **prd2wiki was built first** (before the librarian). It had its own embedder, its own vector store (`internal/vectordb/`), its own search. This was the correct architecture at the time.

2. **The librarian was designed** to replace the wiki's search. Every design document (de836ff, c6525ac, 8634f02, 6ccd407) describes the librarian as THE search backend. 13 audit rounds reviewed these documents.

3. **The audits reviewed documents, not code.** No audit round verified that the wiki's local vector store was actually removed. The code was never checked against the design.

4. **Phase 3a-3d was implemented** by adding librarian integration (libclient, syncToLibrarian, .link write-back) AS AN ADDITION to the existing code. The old vector store was left in place. Nobody removed it because nobody was told to remove it — the plan said "wire libclient" not "remove vectordb."

5. **The implementing agent** saw existing working code (vector store, embedder) and left it alone. They added the new code alongside. This is the "Premature Builder" anti-pattern from the steward rules — building before understanding what already exists.

6. **I (Claude) reviewed and merged all of this** without catching that the old search pipeline was still active. I focused on whether the new code worked, not whether the old code was removed.

7. **The result:** Two parallel search pipelines, double embedding, TEI crashes from the wiki's rebuild, poor search quality, and the librarian's search sitting unused. None of this was in any document because nobody knew it was happening.

---

## Part 4: What Needs to Be Done

### Step 1: Add `MemorySearch` to libclient

**File:** `internal/libclient/client.go`

Add a method that calls the librarian's `memory_search` MCP tool over the unix socket. Returns page UUID, score, title. This is the missing piece that connects wiki search to the librarian.

### Step 2: Rewrite `Librarian.Search()` to call libclient

**File:** `internal/librarian/librarian.go`

Current: `l.vstore.Search()` → scans JSON array.
New: `l.libClient.MemorySearch()` → calls librarian via socket → LanceDB BM25+vector+RRF.
Fallback: if libclient is nil or call fails, fall back to SQLite FTS (title/tag matching).

### Step 3: Remove local embedding on write

**File:** `internal/librarian/librarian.go`

Remove the `indexInVectorStore()` call from the `submit()` method (line 469-471). The librarian receives the page via `syncToLibrarian` and handles its own embedding. The wiki should not embed.

### Step 4: Remove vstore from Librarian struct

**File:** `internal/librarian/librarian.go`

Remove `vstore *vectordb.Store` field. Remove it from `New()` signature. Update all callers.

### Step 5: Remove vector store creation and rebuild from app.go

**File:** `internal/app/app.go`

Remove: embedder creation, `vectordb.NewStore()`, `LoadFromDisk()`, `SetPersistPath()`, vector rebuild goroutine, embedding profile store, `VStore` from App struct.

### Step 6: Remove wiki embedder config

**File:** `config/prd2wiki.yaml`

Remove the `embedder:` section. The wiki doesn't need an embedder.

### Step 7: Delete the vectordb package

**Files:** `internal/vectordb/store.go`, `record.go`, `store_test.go`

### Step 8: Delete runtime vector data

**File:** `data/vectors/pages.json`

### Step 9: Update web search handler

**File:** `internal/web/search.go`

Remove the vector search path that calls `lib.Search()` → local store. Replace with `lib.Search()` → librarian. The FTS fallback stays.

### Step 10: Update API search handler

**File:** `internal/api/search.go`

Same change — `lib.Search()` now goes through the librarian, not the local store.

### Step 11: Update tests

Remove tests that depend on the local vector store. Add tests that verify search calls the librarian.

### Step 12: Update dedup

**File:** `internal/librarian/dedup.go`

DedupDetector uses `vectordb.Store`. Either wire it to the librarian or disable it (it was a stub anyway — "Not yet implemented" in the plan).

### Step 13: Verify end-to-end

- Start librarian + wiki
- Edit a page
- Search for it
- Confirm search goes through librarian (check librarian logs for memory_search call)
- Confirm wiki does NOT connect to TEI
- Confirm wiki does NOT create/load data/vectors/pages.json
- Confirm wiki starts in <10 seconds (no vector rebuild)

---

## Part 5: Dependencies and Risks

### The librarian MUST be running for vector search

If the librarian is down, search degrades to SQLite FTS (title, tags, metadata). No semantic/vector search. This is acceptable — it was the design from the start. FTS covers the common case (searching by title or tag).

### The librarian must have pages indexed

Pages get indexed when:
- `syncToLibrarian` fires on page edit (each edit sends full content)
- Bulk import sends all pages from git history

If the librarian has never received pages (fresh install, librarian wiped), vector search returns nothing. FTS still works.

### TEI must be running for the LIBRARIAN (not the wiki)

The wiki no longer connects to TEI. Only the librarian does. If TEI is down, the librarian still works (BM25 lexical search only, no vectors). The wiki doesn't know or care about TEI.

### Librarian process management

The librarian needs to stay running. Currently it's a manual `nohup` process. Needs a systemd unit or process monitor. This is BUG-011 (still open).

---

## Part 6: What the Reviewer Should Check

**This plan MUST be examined for missing steps.** Specifically:

1. **Is there any other code path that reads from `internal/vectordb/`?** Grep the codebase. If anything besides the listed files uses it, that's a blocker.

2. **Is there any other code path that calls the wiki's own embedder?** If any handler embeds content locally, that's a blocker.

3. **Does `syncToLibrarian` actually send all the data the librarian needs for search?** Check that namespace, page_uuid, content, and metadata (title, tags, type) are all sent. If any are missing, the librarian can't search properly — that's a blocker.

4. **Does the librarian's `memory_search` MCP tool return enough data for the wiki to render results?** Check that page_uuid, score, and optionally title/snippet are in the response. If the wiki needs fields that the librarian doesn't return, that's a blocker.

5. **Are there any pages that exist in git but were never sent to the librarian?** After migration, all pages need to be bulk-imported into the librarian. If this hasn't happened, vector search returns nothing even with the librarian running.

6. **Does the SQLite FTS fallback actually work for the common search case?** Test: search for a page by title. If FTS doesn't find it, the degraded mode is broken — that's a blocker.

7. **Is there any test that asserts the existence of `internal/vectordb/`?** If so, those tests need updating before the package can be deleted.

8. **Does the embedder config in `prd2wiki.yaml` affect anything besides the vector store?** If other code reads `embedder.endpoint` for non-search purposes, removing the config would break it — that's a blocker.

---

## Gate (implementation is not done until ALL of these are true)

- [ ] `internal/vectordb/` package deleted from repo
- [ ] `data/vectors/pages.json` not created on startup
- [ ] Wiki does NOT connect to TEI on startup
- [ ] Wiki does NOT embed pages locally on write
- [ ] `Librarian.Search()` calls libclient → pippi-librarian → LanceDB
- [ ] Search falls back to SQLite FTS when librarian is down
- [ ] Wiki starts in <10 seconds (no vector rebuild)
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] End-to-end: edit page → search finds it via librarian
- [ ] Librarian logs show `memory_search` calls when wiki searches
- [ ] TEI connection only in librarian logs, NOT in wiki logs

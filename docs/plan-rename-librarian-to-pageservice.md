# PLAN: Rename `Librarian` to `PageService` — Responsibility Diagram and Refactor

**Date:** 2026-04-12
**Status:** DRAFT — part of the local vector store removal effort
**Scope:** `internal/librarian/` package in prd2wiki
**Depends on:** Executes alongside or immediately after `plan-remove-local-vectordb.md`

## What `Librarian` Does Today

### Struct fields

```
Librarian
├── repo        *git.Repo           — git bare repo (read/write pages)
├── indexer     *index.Indexer      — SQLite FTS (index pages for listing/filtering)
├── vstore      *vectordb.Store     — LOCAL JSON vector store (REMOVING)
├── vocab       *vocabulary.Store   — term normalization
├── libClient   *libclient.Client   — pippi-librarian RPC client (unix socket)
└── treeHolder  *tree.IndexHolder   — tree index (.link/.uuid files)
```

### Methods and who calls them

```
Submit(ctx, SubmitRequest) → SubmitResult
  Called by: api/tree_api.go (create + update), api/pages.go (create)
  Does:
    1. Validate frontmatter (schema.Validate)
    2. Normalize tags + body (if conform/integrate intent)
    3. Write page to git (repo.WritePageWithMeta)
    4. Index in SQLite FTS (indexer.IndexPage)
    5. Embed locally in JSON vector store (indexInVectorStore) ← REMOVING
    6. Sync to pippi-librarian async (maybeSyncToLibrarian)
       → calls libclient.MemoryStore over socket
       → writes .link line 2 with returned mem_ ID

Search(ctx, project, query, limit) → []SearchResult
  Called by: api/search.go, web/search.go
  Does:
    1. Normalize query via vocabulary
    2. Search local JSON vector store (vstore.Search) ← REPLACING with libclient.MemorySearch

FindSimilar(ctx, project, pageID, limit) → []SearchResult
  Called by: NOBODY (no handler calls it — dead code path)
  Does:
    1. Search local vector store for similar pages

RemoveFromIndexes(pageID) → error
  Called by: api/tree_api.go (delete handler)
  Does:
    1. Remove from SQLite FTS (indexer.RemovePage) ← KEEP
    2. Remove from JSON vector store (vstore.RemovePage) ← REPLACING with libclient.MemoryDelete

RebuildVectorIndex(ctx, project, branch) → (int, error)
  Called by: app.go (startup rebuild goroutine) ← REMOVING entirely

indexInVectorStore(ctx, project, fm, body) → error
  Called by: submit() async goroutine ← REMOVING

maybeSyncToLibrarian(ctx, req, res)
  Called by: submit() at end
  Does:
    1. Check if libClient and treeHolder exist
    2. Resolve pageUUID + projectUUID
    3. Launch async goroutine → runSyncToLibrarian

runSyncToLibrarian(req, pageUUID, projectUUID, commitHash)
  Does:
    1. Build metadata map (title, type, status, tags, author, repo, commit)
    2. Call libclient.MemoryStore over socket
    3. Write returned mem_ ID to .link line 2
```

## What `Librarian` Does After the Vector Store Removal

```
PageService (renamed from Librarian)
├── repo        *git.Repo           — git bare repo
├── indexer     *index.Indexer      — SQLite FTS
├── vocab       *vocabulary.Store   — term normalization
├── libClient   *libclient.Client   — pippi-librarian RPC client
└── treeHolder  *tree.IndexHolder   — tree index

Methods:
  Submit(ctx, SubmitRequest) → SubmitResult
    1. Validate frontmatter
    2. Normalize (if conform/integrate)
    3. Write to git
    4. Index in SQLite FTS
    5. Sync to pippi-librarian async (MemoryStore)
    6. Write .link line 2

  Search(ctx, project, query, limit) → []SearchResult
    1. Normalize query via vocabulary
    2. Resolve project repo key → namespace (wiki:{uuid})
    3. Call libclient.MemorySearch → pippi-librarian → LanceDB
    4. On error: return error (handlers fall back to FTS)

  FindSimilar(ctx, project, pageID, limit) → []SearchResult
    1. Call libclient.MemorySearch with page content as query
    2. On error: return empty

  RemoveFromIndexes(pageID) → error
    1. Remove from SQLite FTS (indexer.RemovePage)
    2. Get mem_ ID from .link line 2 via treeHolder
    3. Call libclient.MemoryDelete async (if ID exists)

  DELETED: RebuildVectorIndex — no local vector store
  DELETED: indexInVectorStore — librarian handles embedding
```

## Why Rename

The struct is called `Librarian` but it is NOT the librarian. The librarian is pippi-librarian — a separate service in a separate repo that manages LanceDB, embeddings, versioning, and search.

This wiki-side struct is a **page service** — it orchestrates page lifecycle (create, edit, search, delete) and delegates to:
- **git** for content storage
- **SQLite** for metadata indexing
- **pippi-librarian** for vector search and versioning (via libclient)
- **tree** for filesystem organization

Calling it `Librarian` causes confusion:
- "lib.Search()" — is that the librarian searching, or the wiki calling the librarian?
- "librarian.New()" — is that creating a librarian instance or a wiki service?
- `internal/librarian/` — is that the librarian's code or the wiki's?

After rename:
- `pageservice.New()` — creates the wiki's page service
- `svc.Search()` — the page service searching (which internally calls the librarian)
- `internal/pageservice/` — clearly the wiki's orchestration layer

## How the System Is Supposed to Look

```
┌─────────────────────────────────────────────────────┐
│ prd2wiki (this repo)                                │
│                                                     │
│  HTTP handlers (api/, web/)                         │
│       │                                             │
│       ▼                                             │
│  PageService (internal/pageservice/)                │
│       │         │           │          │            │
│       ▼         ▼           ▼          ▼            │
│    git.Repo   SQLite    libclient   tree.Index      │
│   (content)   (meta)    (RPC)      (.link/.uuid)    │
│                            │                        │
└────────────────────────────│────────────────────────┘
                             │ unix socket
                             ▼
┌─────────────────────────────────────────────────────┐
│ pippi-librarian (separate repo, separate process)   │
│                                                     │
│  MCP server                                         │
│       │                                             │
│       ▼                                             │
│  MemoryStore / SearchWiki / DeleteWiki               │
│       │              │                              │
│       ▼              ▼                              │
│    LanceDB         TEI                              │
│  (records +      (embedder)                         │
│   vectors)                                          │
└─────────────────────────────────────────────────────┘
```

## Rename Steps

### Step 1: Rename package
- `mv internal/librarian/ internal/pageservice/`
- Update all import paths: `"github.com/frodex/prd2wiki/internal/librarian"` → `"github.com/frodex/prd2wiki/internal/pageservice"`

### Step 2: Rename type
- `Librarian` → `PageService` in all files in the package
- `New()` stays `New()` (or becomes `NewPageService()` if clarity needed)

### Step 3: Rename variables in callers
- `lib` → `svc` or `pageSvc` in handlers
- `librarians` map → `services` or `pageServices` in app.go
- `projectLibrarian()` → `projectService()` in api/server.go

### Step 4: Rename related types
- `SubmitRequest` stays (it's about submission, not about being a librarian)
- `SubmitResult` stays
- `SearchResult` stays
- `Option` stays

### Step 5: Update MCP tools
- `s.client.CreatePage` references — check if MCP client uses "librarian" terminology

### Step 6: Update tests
- All test files in the package
- All test files that import the package

### Step 7: Update docs
- Constraint files, plan docs, wiki pages that reference `internal/librarian/`
- README if it references the package

## When to Do This

**During the vector store removal.** We're already:
- Removing `vstore` from the struct
- Changing the `New()` signature
- Rewriting `Search()` and `RemoveFromIndexes()`
- Updating all callers

The rename is incremental on top of changes we're already making. Doing it separately later means touching all the same files twice.

## Gate

- [ ] Package renamed to `internal/pageservice/`
- [ ] Type renamed to `PageService`
- [ ] All callers updated
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] No remaining references to `internal/librarian/` (grep verified)

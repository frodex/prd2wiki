# Stepped review: `plan-remove-local-vectordb.md` (dual-repo, dual commit)

**Instruction:** This file is a **new** artifact. It does **not** modify `plan-remove-local-vectordb.md` or other pre-existing docs.

---

## Step 0 — Scope table (required for every review)

| Field | Value |
|--------|--------|
| **Plan reviewed** | `docs/plan-remove-local-vectordb.md` (workspace copy) |
| **prd2wiki root** | `/srv/prd2wiki` |
| **prd2wiki commit** | `a33083520c47e5b68faf1d4c88b89b474b261eb6` |
| **pippi-librarian root** | `/srv/pippi-librarian` |
| **pippi-librarian commit** | `cc96c4bd10aa6b0cec0669ae9bf6c795dd3e9291` (plan short: `cc96c4b`) |
| **Method** | Static read of Go sources in both trees; no services run |

---

## Step 1 — Confirm plan Part 1 (librarian architecture) vs pippi-librarian

| Claim | Verification | Result |
|--------|----------------|--------|
| RRF fusion **k=60** | `internal/librarian/table.go`: `rrfFuse(..., 60)` in `SearchWithFilter` / `SearchAmong` | **Match** |
| **`WikiSearchHit` fields** | `internal/librarian/memsvc.go`: `PageUUID`, `RecordID`, `Title`, `Snippet`, `Score`, `HistoryCount` | **Match** |
| **`memory_search` serializes hits** | `cmd/pippi-librarian/main.go`: handler builds `matches` with those keys | **Match** |

---

## Step 2 — Confirm plan Part 10 (metadata dropped) vs pippi-librarian

| Claim | Verification | Result |
|--------|----------------|--------|
| MCP schema includes **`metadata`** | `main.go` `memory_store` `InputSchema` includes `"metadata"` | **Match** |
| Handler does **not** pass metadata to store | Handler: `StoreWiki(ctx, namespace, pageUUID, content)` only — no `args["metadata"]` | **Match — BLOCKER as plan states** |
| **`StoreWiki` signature** | `internal/librarian/memory.go`: `(ctx, namespace, pageUUID, content string)` | **Match** |
| **`MemoryRecord` lacks title/tags** | Struct has `Content`, `PageUUID`, versioning; no `page_title` / tag fields | **Match** |
| **Embedding uses body only** | `EmbeddingText()` returns `m.Content` | **Match** |
| **Lance schema** | `memory_lance.go` `buildMemoryLanceSchema`: no dedicated `page_title` / `page_tags` columns; has `meta_json` + `vector` | **Match** — Part 10’s “add columns” is directionally right; **`meta_json`** could alternatively hold metadata if wired |

**Nuance:** `SearchWiki` sets result **`Title`** from **`firstLineTitle(rec.Content)`** (`memory.go`), not from MCP metadata. Dropped metadata still yields a **title string in API results**, derived from **first line of body**, which may differ from wiki frontmatter title.

---

## Step 3 — Confirm plan Part 2 / prd2wiki inventory vs code

| Area | Verification | Result |
|------|----------------|--------|
| **`app.go` `New()`** embedder + vstore + disk + rebuild | `internal/app/app.go` | **Match** |
| **`Librarian.Search`** vocab + `vstore.Search` fusion | `internal/librarian/librarian.go` + `internal/vectordb/store.go` (0.7/0.3) | **Match** |
| **API vs web search** | `internal/api/search.go` parallel merge vs `internal/web/search.go` vector-first | **Match** |
| **Dead dedup** | `submit()` never reads `flags.dedup` | **Match** |
| **`libclient.New`** non-nil client on dial failure + misleading “sync enabled” | `internal/libclient/client.go` + `internal/app/app.go` | **Match** |
| **`libclient`**: `MemoryStore` only | `internal/libclient/client.go` | **Match — MemorySearch/MemoryDelete absent** |

**Repo-key vs namespace:** Local vstore filters by **`project`** (repo key); **`runSyncToLibrarian`** uses **`wiki:`+projectUUID**. Plan Step 2 namespace mapping is **required** — verified conceptually via `tree_api` passing `ProjectUUID` to sync and search using repo key for librarian lookup.

---

## Step 4 — Cross-repo blocker: `memory_delete` vs plan Step 11b

| Source | What it says |
|--------|----------------|
| **Plan Step 11b** | `MemoryDelete()` takes **page_uuid**, delete by page_uuid |
| **`schema.d/wiki.yaml`** | `memory_delete` **required**: **`id`** (mem_ id) |
| **`main.go` handler** | `DeleteWiki(ctx, fmt.Sprint(args["id"]), ...)` |
| **prd2wiki `treeDeletePage`** | `RemoveFromIndexes(ent.Page.UUID)` — **page UUID**, not mem_ id |

**Conclusion:** Plan text for wiki-side delete **does not match** the librarian tool. Implementers need **`memory_get` by page_uuid → record_id**, or **head id from `.link`**, then **`memory_delete` by id**, or a **new librarian operation**.

**Severity:** **BLOCKER** for tree delete → librarian orphan strategy until resolved.

---

## Step 5 — Query normalization parity (both repos)

| Location | Behavior |
|----------|----------|
| **prd2wiki** `Librarian.Search` | `l.vocab.Normalize` per query token |
| **pippi-librarian** `SearchAmong` / `SearchWithFilter` | `NormalizeSemantic(query)` for embedding; different from wiki vocab |

**Conclusion:** “Send wiki-normalized query” ≠ “same as librarian normalization.” Plan flags parity; treat as **design decision**, not automatic equivalence.

---

## Step 6 — Internal plan consistency (document-only)

| Section | Content |
|---------|---------|
| **Part 5** | Option **B** + **Part 10 librarian first**; metadata currently dropped |
| **Part 7 Step 4** | Still describes **prepending** in **`runSyncToLibrarian`** (wiki-side enrichment) |

**Conclusion:** If **B + Part 10** is authoritative, **Step 4** must be reconciled with “librarian owns prefixing after metadata exists” to avoid **double enrichment** or contradictory instructions.

---

## Step 7 — prd2wiki grep completeness (non-test production paths)

All `.go` files referencing **`vectordb` / `embedder` / `libclient`** in this pass:

- `internal/app/app.go`, `internal/librarian/librarian.go`, `internal/libclient/client.go`, `internal/embedder/*`, `internal/vectordb/*`, `internal/librarian/normalizer.go`, `internal/librarian/dedup.go`
- **`cmd/`**: no runtime embedder except string reference in `cmd/backfill/main.go` to a **markdown** plan path

**Conclusion:** Part 9 embedder checklist (“MCP reads config”) is **low risk** for `prd2wiki-mcp` from this grep pattern; still verify **any** binary that loads `config/prd2wiki.yaml` before deleting **`embedder:`**.

---

## Step 8 — Verdict

| # | Finding |
|---|---------|
| 1 | **Part 10** metadata / store / Lance claims are **substantiated** at **pippi `cc96c4b`**. |
| 2 | **Part 1** RRF / `WikiSearchHit` / `memory_search` shape **substantiated**. |
| 3 | **`memory_delete` by page_uuid** as written in the plan **conflicts** with librarian **`id`-only** delete — **fix plan or fix API**. |
| 4 | **Part 5 Option B** vs **Step 4 wiki prepend** needs **single coherent** instruction. |
| 5 | **Vocab vs `NormalizeSemantic`** remains a **parity** topic across repos. |

---

## Step 9 — Related artifacts (do not edit)

| File | Role |
|------|------|
| `docs/plan-remove-local-vectordb.md` | Canonical plan — **not modified** by this review file |
| `docs/plan-remove-local-vectordb-REVIEW-5-stepped-dual-repo.md` | **This file** — stepped dual-repo review |

---

*Re-run `git rev-parse HEAD` in each repo after pulls; update Step 0 when reusing.*

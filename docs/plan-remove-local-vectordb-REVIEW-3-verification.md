# Third pass: `plan-remove-local-vectordb.md` vs prd2wiki source

**Scope of this file:** Findings and suggestions live **here only**. The canonical plan at `docs/plan-remove-local-vectordb.md` is **not** edited by review passes unless the author explicitly asks.

## Document under review (exact)

| Field | Value |
|--------|--------|
| **Path** | `/srv/prd2wiki/docs/plan-remove-local-vectordb.md` |
| **Repo root** | `/srv/prd2wiki` |
| **Remote** | `https://github.com/frodex/prd2wiki.git` (`git remote get-url origin` on this machine) |
| **Branch** | `main` |
| **Commit** | `b8f760c81ff2fcf27f7adb40d414cc2ffb158e26` |

Verification is **static** (read/grep only). **pippi-librarian** was **not** opened; Part 1 bullet claims there are **not** re-verified here.

---

## Confirmed: plan matches this repo

- Inventory rows for `app.go` `New()`, vstore wiring, `librarian` `Search` / `FindSimilar` / `RemoveFromIndexes` / `indexInVectorStore` / async submit, API parallel merge, web vector-first + FTS when empty, `normalizer` + `TextChunk`, dead dedup, `libclient.New` + misleading “sync enabled” — **align with** `internal/app/app.go`, `internal/api/search.go`, `internal/web/search.go`, `internal/librarian/librarian.go`, `internal/libclient/client.go` at the commit above.
- **`VStore` on `App`:** only assigned in `app.go`; **no** other `.VStore` references — “no external readers” holds.
- **`MemorySearch` / `MemoryDelete`:** not in `internal/libclient/client.go` — plan “MISSING” is still accurate.
- **Embedder readers:** `grep` of `embedder` / `Embedder` in `cmd/**/*.go` shows **no** matches (aside from unrelated string in `cmd/backfill/main.go` pointing at a **markdown** path). Config still has `embedder:` in `config/prd2wiki.yaml`; app loads it — plan removal is coherent.

---

## Issues / gaps (plan or implementation risk)

### 1. Inventory row 9 understates local `Search` scoring

Table says local path is “JSON array **cosine scan**.” In **`internal/vectordb/store.go`**, `Search` uses **0.7×cosine + 0.3×keyword** on stored text (Part 5b in the plan already describes this). **Suggestion:** one line in the inventory: “fused cosine + keyword, not cosine-only.”

### 2. Row order in inventory

Rows **#19** and **#20** are **out of numeric order** (#20 appears before #19). Cosmetic; fix for readability.

### 3. Phase 1 Step 2 — “FTS fallback” inside `Librarian.Search`

The plan says when `libClient` is nil or `MemorySearch` fails, fall back to SQLite FTS from **`Librarian.Search`**.

**Fact:** `Librarian` has **`indexer`** (writes) but **no** `*index.Searcher`. FTS today is invoked from **`api/search.go`** and **`web/search.go`**, not from `librarian`.

**Implication:** Implement either:

- **A)** `Librarian.Search` **returns an error** when librarian search is unavailable, and keep FTS-only / merge behavior **in handlers** (API already runs FTS in parallel; on `lib.Search` error the vector leg is skipped but FTS remains). **Web:** `err != nil` → `items` stays empty → existing FTS fallback runs.  
- **B)** Inject **`Searcher`** into `Librarian` and duplicate FTS in one place (plan should spell this out if intended).

As written, Step 2 is **ambiguous / slightly wrong** unless interpreted as “return error, let callers degrade.”

### 4. Steward constraint “log when FTS fallback activates”

**Part 4** says FTS fallback must **log**. **`internal/api/search.go`** and **`internal/web/search.go`** have **no** `slog` calls on the search paths — **no logging today**. Treat as **new work** when aligning with the constraint, not “already satisfied.”

### 5. `app.go` comment drift

Line ~199 comment says “**LlamaCpp**” while code uses **`NewOpenAIEmbedder`** (OpenAI-compatible / TEI). Harmless for behavior; worth a cleanup when touching embedder removal.

### 6. Part 9 checklist — grep may miss `PageEmbedding` / `TextChunk`

Suggested grep uses `vstore|vectordb|FindSimilar|RemovePage|IndexPage|EmbedBatch`. **`record.go`** and types like **`TextChunk`** / **`PageEmbedding`** may not match. **Suggestion:** extend pattern or add a second grep for `TextChunk|PageEmbedding`.

### 7. Rollback section timing

“~5 min” re-embed is **environment-dependent** (corpus size, TEI speed). Fine as ballpark; label as **estimate**.

### 8. Cross-repo claims (unchanged)

Part 1 “Verified against pippi-librarian source” requires a **pippi-librarian** path + commit per the plan’s own **Codebase scope** rules — **not** satisfied inside this file alone.

---

## Related files (this effort)

| Path | Role |
|------|------|
| `/srv/prd2wiki/docs/plan-remove-local-vectordb.md` | Canonical plan (read-only for reviewers) |
| `/srv/prd2wiki/docs/plan-remove-local-vectordb-RESPONSE-verification.md` | First verification |
| `/srv/prd2wiki/docs/plan-remove-local-vectordb-REVIEW-2-deep.md` | Second pass |
| `/srv/prd2wiki/docs/plan-remove-local-vectordb-REVIEW-3-verification.md` | **This file** — third pass at commit `b8f760c` |

---

*Re-run `git rev-parse HEAD` after pulls; update this document’s commit pin when reusing.*

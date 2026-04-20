# Review: `plan-remove-local-vectordb.md` (fourth pass, persisted from chat)

**Created:** 2026-04-12  
**Source:** Same content as the in-chat deep-dive review; no edits were made to the plan file.

---

## Scope (so comparisons line up)

- **Plan file:** `docs/plan-remove-local-vectordb.md` (workspace copy at time of review).
- **Code verified:** repo root `/srv/prd2wiki` only, commit **`f6250f8842746625e1d640fbd3e2289895f73711`**, branch **`main`**, remote **`https://github.com/frodex/prd2wiki.git`**.
- **pippi-librarian:** Not re-opened in this pass. Part 1’s file/commit references are **claims**; treat them as needing a matching checkout of **`/srv/pippi-librarian`** at the commit named in the plan (e.g. `cc96c4b…`) if you audit cross-repo behavior.

---

## What matches the repo (high confidence)

- **`internal/app/app.go` `New()`** wires **`embedder.NewOpenAIEmbedder` → `vectordb.NewStore` → `LoadFromDisk` / `SetPersistPath`**, shared **`vstore`** across librarians, optional background **`RebuildVectorIndex`** when **`vstore.Count()==0`**, **`App.VStore`**, embedding profile store — all present as described.
- **`Librarian.Search`** uses **`l.vocab.Normalize`** then **`l.vstore.Search`**; **`internal/vectordb/store.go` `Search`** uses **0.7×cosine + 0.3×keyword** (plan inventory / Part 5b align with code).
- **`internal/api/search.go`:** FTS and **`lib.Search`** in **parallel**, merge FTS first then vector IDs — as stated.
- **`internal/web/search.go`:** **`lib.Search` first**; FTS only when **`len(items)==0`** (including **`lib.Search` error** → empty items → FTS). Part 5b’s “not the same as API” holds.
- **`FindSimilar`:** only **`internal/librarian/librarian.go`** + **`vectordb`** tests; **no** API/web routes — aligns with checklist item 5.
- **Dead dedup:** **`flags.dedup`** is set for integrate intent but **`submit()` never reads `flags.dedup`**; no **`DedupDetector`** use in **`submit()`** — plan’s “dead code” story holds.
- **`libclient.New`:** on dial failure returns **`(non-nil Client, error)`**; **`app.go`** still treats **`pippi != nil`** as “sync enabled” — plan’s operator-misleading behavior is accurate.
- **Stale comment:** **`app.go`** still says **“LlamaCpp”** while using **`NewOpenAIEmbedder`** — inventory row 21 is fair.
- **`internal/libclient/client.go`:** **`MemoryStore` only**; no **`MemorySearch` / MemoryDelete`** — “MISSING” still correct.
- **Embedder in `cmd/`:** **`cmd/prd2wiki-mcp`** has no **`embedder`** references; risk of “MCP breaks if YAML embedder removed” is low for this tree, assuming MCP does not load the same config for embedder elsewhere (worth one grep before deleting YAML).

---

## Gaps / risks the plan under-emphasizes (implementation blockers)

### 1. Namespace / project identity: RepoKey vs `wiki:{UUID}`

Today:

- Local vector records filter by **`project`** passed into **`Librarian.Search`**, which comes from the URL / repo key (e.g. **`SubmitRequest.Project`** / tree **`proj.RepoKey`**), and **`indexInVectorStore`** indexes with that same **`req.Project`**.
- **`runSyncToLibrarian`** uses **`namespace := "wiki:" + projectUUID`** where **`projectUUID`** is the **tree project UUID**, not the repo key.

So **memory_store** and **local vstore** do **not** use the same string as the “project” dimension: **repo key vs `wiki:`+UUID**. For **`memory_search`**, the implementation must **resolve repo key → tree `Project.UUID` → namespace `wiki:`+uuid** (e.g. via **`tree.Index.ProjectByRepoKey`** on the handler side or by threading UUID into **`Librarian`**). The plan’s steps talk about **`MemorySearch()`** but not this **mapping**; without it, search can hit the **wrong namespace** or return **empty** results even when data exists.

### 2. `Librarian.Search` vocabulary normalization vs future `memory_search`

The plan already flags parity. In code, **`Search`** normalizes the **query** with **`l.vocab`** before the local store. After switching to **`memory_search`**, you must decide: **drop wiki-side normalization**, **duplicate it before the RPC**, or **ensure the librarian applies the same rules**. Leaving this implicit will look like “search regressed” for some queries.

### 3. Phase ordering vs `TextChunk` / `vectordb` package

The plan puts **Step 10** (drop **`vstore`** from **`Librarian`**) before **16b** (move **`TextChunk`**). That can still work **if** the **`internal/vectordb`** package remains until **Step 17** for **`normalizer.go`** / tests — but it is easy to delete **`vectordb`** too early. Treat **“package exists until `TextChunk` moved”** as an explicit ordering invariant for implementers.

### 4. Performance table (Part 4)

- **Startup 10–120s:** Plausible when **`vstore.Count()==0`** triggers a full **background** rebuild; not measured in this review.
- **“Write latency 60–120s”:** **`submit()`** returns after **git + SQLite index**; **local embedding runs in a goroutine** after the response. So **HTTP write latency** is **not** 60–120s from embedding in the common case — the table likely mixes **background indexing** with **request latency**. Split “user-visible submit time” vs “time until local vector index reflects the edit.”

### 5. Part 8 rollback

Rollback assumes **old code + `pages.json`** still exist. If **`data/vectors/`** was removed without a backup, rollback still needs a **full re-embed**; the plan mentions that. “~5 min” remains an **estimate**.

### 6. Cosmetic / doc hygiene (non-blocking)

- Inventory rows **#20** appear **before** **#19** in the plan table; numbering is confusing.
- **Step 11** “compactor or next reconcile” is a **pippi-side** assumption; verify in **pippi-librarian** before relying on it.

---

## Verdict

The plan is **substantially aligned** with **`prd2wiki` @ `f6250f`** on wiring, dead dedup, web vs API search, libclient quirks, **`TextChunk`**, and missing **`MemorySearch`**. The **largest missing explicit requirement** for a correct cutover is **consistent namespace selection for `memory_search` / `memory_store` (repo key → `wiki:`+project UUID)**, plus a clear story for **query normalization** after **`Librarian.Search`** stops calling the local store.

---

## Related documents

| File | Role |
|------|------|
| `docs/plan-remove-local-vectordb.md` | Canonical plan (not modified by this review file) |
| `docs/plan-remove-local-vectordb-RESPONSE-verification.md` | First verification |
| `docs/plan-remove-local-vectordb-REVIEW-2-deep.md` | Second pass |
| `docs/plan-remove-local-vectordb-REVIEW-3-verification.md` | Third pass |
| `docs/plan-remove-local-vectordb-REVIEW-4-in-chat.md` | **This file** — chat review persisted |

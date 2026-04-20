# Response: Code verification vs `plan-remove-local-vectordb.md`

**From:** Code verification  
**To:** Plan author / reviewers  
**Date:** 2026-04-12  
**Re:** `docs/plan-remove-local-vectordb.md` — source-grounded compliance check  

## Codebase under review

All verification in this document applies only to:

| Field | Value |
|--------|--------|
| **Root path** | `/srv/prd2wiki` |
| **Remote** | `https://github.com/frodex/prd2wiki.git` |
| **Scope** | Files in this clone only — not pippi-librarian, not any other repo, not sibling `/srv/*` trees unless stated |

**Note:** Wiki runtime was down during this review; verification is **against the git tree** above, not a live stack. Re-verify after `git pull` (commit may differ from `b522b6a` used for the deep review addendum).

### Related documents (full paths)

Repo root: `/srv/prd2wiki`.

| File | Role |
|------|------|
| `docs/plan-remove-local-vectordb.md` | **Canonical plan** — verify independently; reviewer docs do not modify it. |
| `docs/plan-remove-local-vectordb-RESPONSE-verification.md` | **This file** — verification vs plan claims. |
| `docs/plan-remove-local-vectordb-REVIEW-2-deep.md` | Second pass: blockers, web vs API search, `TextChunk`/vectordb delete, `prd2wiki-import` absent in-repo, etc. |

---

## Did the prior review “verify to the spec”?

The earlier steward-rules review assessed **process and document shape** (constraints, anti-patterns, co-sign gaps). It did **not** systematically walk **prd2wiki** source line-by-line against the plan’s inventory. **This document supplies that second layer:** what matches, what drifted, and what is **undocumented behavior / dead code** that agents may have mistaken for product features.

---

## Verified: plan matches current code

| Plan claim | Source check |
|------------|----------------|
| Local `vectordb.Store` wired from `app.go` | Confirmed: `vectordb.NewStore(emb)`, `LoadFromDisk`, `SetPersistPath`, passed into `librarian.New`. |
| `Librarian.Search` uses `l.vstore.Search` | Confirmed: `internal/librarian/librarian.go` → `l.vstore.Search`. |
| `FindSimilar` uses `l.vstore.FindSimilar` | Confirmed; **no** `FindSimilar` call sites in `internal/api` or `internal/web` (grep). |
| `RebuildVectorIndex` + background rebuild when store empty | Confirmed: `app.go` errgroup/background when `vstore.Count() == 0`. |
| `libclient.MemoryStore` exists; **`MemorySearch` does not** | Confirmed: `internal/libclient/client.go` has `MemoryStore` only. |
| `App.VStore` field | Present on `App`; **no** non-test reads of `VStore` outside `app.go` in this tree (grep). |

---

## Corrections: plan is wrong or stale on these points

### 1. Embedder naming vs comments (`app.go`)

- **Plan / inventory** implies TEI / Llama-style wiring in places; **`app.go` default `embCfg.Type` is `"openai"`** (OpenAI-compatible `/v1/embeddings` client).
- The **comment** at `app.go` still says *“try real LlamaCpp”* but the code constructs **`embedder.NewOpenAIEmbedder`** only. That is **stale documentation in code**, not LlamaCpp.

**Implication:** Any operational doc that says “wiki talks to TEI” must be reconciled with **what the client actually is** (`embedder_openai.go`).

### 2. `DedupDetector` and `integrate` intent — **not in the submit path**

The plan treats **DedupDetector** as an active integrate-time quality gate (`librarian.go` “489–496” in the plan).

**Current `submit()` in `internal/librarian/librarian.go`:**

- Sets `submitFlags{..., dedup: true}` for **`IntentIntegrate`** (`submitFlagsForIntent`).
- **`flags.dedup` is never read** anywhere in `submit()`.
- **`DedupDetector` is never constructed or called** in production `librarian.go`.
- `warnings` in `SubmitResult` is always the empty slice in the success path.

So: **dedup is dead wiring** (flag + tests + `dedup.go`), while the comment on the async goroutine still says *“Vector embedding + dedup async”* — **misleading**.

**Implication for the plan:** Removing vectordb does **not** remove a user-visible “dedup warnings on integrate” feature **unless** some other branch reintroduced it; today those warnings **do not exist** in this path. The plan should be updated so implementers do not spend cycles “preserving” behavior that is **not shipped**.

### 3. `libclient.New` signature and startup behavior

The plan text elsewhere may assume `New(socket, key) *Client` only.

**Current code:** `func New(socketPath, apiKey string) (*Client, error)` — **dial check** at creation (`BUG-008` comment). On dial failure it returns **`(non-nil *Client, err)`** — the HTTP client is still constructed.

**`app.go` behavior:** If `err != nil`, it logs `slog.Error(...)` but **does not clear `pippi`**. The next `if pippi != nil` is still true, so **`WithPippiLibrarian` is wired** even when the socket was unreachable at startup.

**Implication:** “Sync enabled” in logs can mean **“client struct exists”**, not **“socket was healthy at startup.”** Every `MemoryStore` still fails at request time until the socket works. This is easy to misread as “feature works” when it is **degraded**. Worth fixing or documenting in the same effort as vectordb removal.

### 4. Line numbers

The plan cites `app.go:199-208`, `librarian.go:469-471`, etc. **Line numbers drift** with every edit. The **inventory table should anchor on function names** (the plan already warns about this; treat line refs as **hints**, not contracts).

---

## Critical gap: `internal/api/search.go` merge behavior (not in Part 2 inventory)

For **non-empty text query**, `searchPages` runs **in parallel**:

1. SQLite **FTS5** (`s.search.FullText`), and  
2. **`lib.Search`** → **local vectordb** (`internal/api/search.go`, two goroutines + `Wait`).

Results are **merged**: FTS rows first, then vector hits (by page ID), with fallback rows if SQLite has no row for a vector id.

**This is central user-visible search behavior.** The removal plan must say explicitly whether **post-migration search** will:

- Call **librarian `memory_search`** for the “vector” leg and still merge with FTS the same way, or  
- Change ranking / coverage (e.g. only pages synced to librarian).

**Risk:** If local vectors are removed and `lib.Search` is switched to librarian **without** handling namespace / missing pages, search can go **empty or stale** for old content — the plan’s Part 8 #4 (“bulk import”) is a **product blocker**, not only a reviewer checkbox.

---

## Undocumented “workarounds” that became semantic (agents should treat as contract until changed)

These are **real in source** but easy to miss if you only read high-level docs:

### A. `indexInVectorStore` prepends title/tags/type to every chunk

`internal/librarian/librarian.go` (`indexInVectorStore`): builds a **`prefix`** from title, tags, and type, then **prepends it to each chunk** before embedding.

Comment in code: without this, vector search “misses” pages where title/tags are the best match.

**This is a relevance contract** for local semantic search. Librarian-side embedding uses **`memory_store` content** as sent today — if that full preprocessing is **not** mirrored in what gets synced, **search behavior diverges** from historical wiki behavior.

### B. Async vector index after git success (`BUG-012/BUG-014`)

Submit returns after git + SQLite; **vector indexing runs in a goroutine**. Combined with async **`maybeSyncToLibrarian`**, there are **two** background pipelines. Removing one without documenting ordering **changes** race semantics (stale search vs librarian).

### C. `libclient` socket dial + “sync enabled” logging

Startup may log librarian enabled even when dial fails, depending on error handling — **operators** infer health from logs; treat log strings as API.

---

## Summary table: plan Part 2 inventory vs this verification

| Plan row / topic | Verdict |
|------------------|---------|
| Embedder = OpenAI-compatible client (not Llama in code) | **Update plan** — match `embedder.NewOpenAIEmbedder` + defaults. |
| Dedup on integrate | **Incorrect** — `flags.dedup` unused; `DedupDetector` not called in `submit()`. |
| `FindSimilar` only internal | **Confirmed** — no API route found. |
| `VStore` unused outside `App` | **Confirmed** (grep). |
| Search = vector only | **Incomplete** — API merges FTS + vector; **must be in plan**. |
| `libclient.MemorySearch` missing | **Confirmed**. |

---

## Recommended edits to the plan (before implementation)

1. **Add `internal/api/search.go`** to the inventory and Phase 1: define merged search behavior after vectordb removal.  
2. **Rewrite Dedup section** to match reality (dead code / tests) or file a separate ticket to **wire** dedup if product wants it.  
3. **Document chunk prefixing** (`indexInVectorStore`) and decide whether **`memory_store` body** should include equivalent metadata for parity with librarian search.  
4. **Replace fragile line numbers** with function anchors throughout.  
5. **Add explicit bulk backfill / coverage** strategy (Part 8 #4) as an implementation phase, not only a reviewer question.  
6. **Align embedder docs** with code (or change code to match docs — product decision).

---

## Closing (your stated pain: undocumented workarounds)

Several behaviors (**chunk prefixing**, **FTS+vector merge**, **async vector goroutine**, **misleading “dedup” comment**, **unused integrate dedup flag**) are exactly the class of “**workaround in code that became semantic**” without being promoted to docs. This verification pass is meant to **surface** them so the removal plan does not repeat the same failure mode: **looks wired in the plan, isn’t in production**, or **differs in behavior** from what operators assume.

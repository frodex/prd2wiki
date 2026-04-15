# Review 7 — Big-picture "cat vs dog" review

**Procedure:** This is a **new** review artifact. It does **not** modify `plan-remove-local-vectordb.md` or any other existing file. The goal is to step back from line-level verification and ask whether the plan achieves what it says it does.

---

## Step 0 — Scope

| Field | Value |
|--------|--------|
| **prd2wiki commit** | `201a10f3da2e595edbde787a64d44cc6e7a8a57c` |
| **pippi-librarian commit** | `cc96c4bd10aa6b0cec0669ae9bf6c795dd3e9291` |

---

## Step 1 — What does the plan say it is?

Title: **"Remove Wiki's Local Vector Store — Use Librarian for All Search"**

Stated intent: delete `internal/vectordb/`, the wiki embedder, and the JSON vector file; route all semantic search through `pippi-librarian` over a unix socket.

---

## Step 2 — What is it actually?

The plan is **two projects wearing one trenchcoat:**

| Project | Where | What |
|---------|-------|------|
| **A** | prd2wiki | Remove local embedder, vectordb package, and JSON store. Rewrite `Librarian.Search` / `FindSimilar` / `RemoveFromIndexes` to call pippi via libclient. Wire namespace mapping. Add FTS-fallback logging. Backfill. |
| **B** | pippi-librarian | Teach `memory_store` to accept and persist metadata. Add Lance columns or use `meta_json`. Enrich embeddings with title/tags. Fix `SearchWiki` title derivation. Optionally support metadata filtering. |

**Project B is declared a BLOCKER for Project A.** Part 10 is 9 subsections of pippi-librarian changes that must land **first**.

**The problem:** This is a **prd2wiki** plan file in a **prd2wiki** repo. It cannot gate or schedule work in a different repository. Part 10 is a **spec for pippi-librarian changes** embedded inside a prd2wiki removal plan. If nobody picks up Part 10, the plan is permanently blocked. If Part 10 is picked up but diverges from the spec here (e.g. different column strategy), this plan's assumptions break silently.

**Recommendation:** Part 10 belongs in pippi-librarian as its own ticket/plan with its own verification gate. This plan should **reference** it by name, not **define** it.

---

## Step 3 — Does the plan remove a local vector store?

**Yes, eventually.** Phase 2 (Steps 7–18) correctly identifies and removes:
- Embedder creation + health check
- `vectordb.NewStore` + disk persistence
- Background rebuild goroutine
- `indexInVectorStore` + async call from `submit()`
- Dead dedup
- `VStore` on `App`, `vstore` on `Librarian`
- Config section
- Package + data files

This is the **strongest** section of the plan. It's been verified 6 times. It's correct.

---

## Step 4 — Does the plan preserve search?

**Only if Part 10 ships.** Without it:

| Scenario | Outcome |
|----------|---------|
| Remove local vectordb, Part 10 **not** shipped | `MemorySearch` calls librarian → librarian searches **body-only** embeddings (no title/tags enrichment) → **ranking regresses** for title/tag queries. `firstLineTitle(rec.Content)` yields result "titles" from first body line, which is often a heading, not the frontmatter title. |
| Remove local vectordb, Part 10 **shipped** | `MemorySearch` → librarian searches enriched embeddings (title+tags prepended) → ranking is different (RRF vs 0.7/0.3 fusion) but **enrichment parity** exists. Title from metadata. Reasonable. |

The plan **knows this** (Part 5 says BLOCKER). But the **execution dependency** is in a different repo with no owner named.

---

## Step 5 — What does search actually look like today?

The plan focuses on `Librarian.Search` as "the" search path. But the wiki has **three** search surfaces, not two:

| Surface | What it does | Uses `Librarian`? |
|---------|-------------|-------------------|
| **API text search** (`api/search.go`) | Parallel FTS + `lib.Search`, merge | Yes |
| **Web text search** (`web/search.go`) | `lib.Search` first, FTS if empty | Yes |
| **Structured filter** (ByType, ByStatus, ByTag) | SQLite index only — `api/search.go`, `web/search.go`, `api/pages.go`, `web/list.go`, `web/home.go` | **No** |

The **third surface is 100% SQLite** and is **untouched** by this plan. That's correct behavior — structured filters don't need vectors. But Part 10.5 says "SearchWiki should support filtering by type, status, tags." **Why?** The wiki already has that via SQLite. Adding it to the librarian duplicates capability that already works. Unless the intent is to eventually **remove SQLite FTS too**, Part 10.5 is scope creep that doesn't serve the plan's stated goal.

---

## Step 6 — `ChunkByHeadings` is dead after this plan

`ChunkByHeadings` (in `normalizer.go`) is called from **one place**: `indexInVectorStore` (`librarian.go:390`).

Step 7 removes `indexInVectorStore`. After that, **nothing calls `ChunkByHeadings`**. The function, its tests (`normalizer_test.go`), and the `vectordb.TextChunk` type it returns are all **dead code** post-migration.

The plan has **Step 16b** ("move `TextChunk` to librarian package") — but there's no reason to move a type that nothing uses. The simpler action is:

1. Delete `indexInVectorStore` (Step 7)
2. Delete `ChunkByHeadings` + tests (not in plan)
3. Delete `internal/vectordb/` including `TextChunk` (Step 17)
4. No Step 16b needed

The plan builds infrastructure (move a type) for something that has no consumer.

---

## Step 7 — The `Librarian` struct is doing too many things

After this plan, `Librarian` will:
- Orchestrate page submission (validate, normalize, write git, index SQLite)
- Sync to pippi-librarian (async `MemoryStore`)
- Search via pippi-librarian (`MemorySearch`)
- Delete from pippi-librarian (`MemoryDelete`)
- Update `.link` files
- Hold vocabulary store (for query normalization)

It started as a **page submission** orchestrator. It's becoming a **pippi-librarian RPC client** that also does page submission. The plan doesn't address this, but it's worth noting for the implementer: after this work, consider whether `Librarian` should be split or renamed.

---

## Step 8 — Backfill creates unnecessary version churn

`StoreWiki` creates a **new `mem_` head** and demotes the old one to "superseded" on **every** call (`memory.go:140–189`). There is no "upsert if content unchanged" or idempotency check.

Step 19 says "send all pages to librarian." Pages that were **already synced** via `maybeSyncToLibrarian` will get a **new version** even if content is identical. For a wiki with N pages where M were already synced, backfill creates M unnecessary version rows.

This isn't catastrophic (superseded rows are filtered from search), but it's **wasteful** and makes history noisy. The plan should note this or suggest an idempotency strategy (e.g. skip if content hash matches current head, or call `memory_get` first).

---

## Step 9 — `api/pages.go` DELETE path doesn't use `Librarian`

There are **two** delete paths:

| Path | Code | Uses `Librarian`? |
|------|------|-------------------|
| **Tree API** | `tree_api.go:365` → `lib.RemoveFromIndexes(uuid)` | **Yes** — plan Step 11 covers this |
| **Pages API** | `pages.go:212` → `s.indexer.RemovePage(id)` | **No** — calls `indexer` directly, no `vstore`, no librarian |

The plan's inventory and steps focus on `RemoveFromIndexes` (the `Librarian` method). But the Pages API delete at `pages.go:212` **also** removes pages and **never** tells the librarian. This path **already** orphans librarian records today (it doesn't call `maybeSyncToLibrarian` or `vstore.RemovePage`). The plan doesn't make this worse, but it also doesn't fix it. Worth flagging as a **pre-existing orphan source** that the plan should either fix or explicitly defer.

---

## Step 10 — Verdict: is this a cat or a dog?

**It's a dog, but it's a dog that can't walk without a leash from another repo.**

The prd2wiki-side work (Phases 1–3) is sound: remove local embedder + vectordb, route search through libclient, fall back to FTS, backfill. Six reviews have beaten the details into shape.

**The fundamental risk is not in any detail. It's structural:**

1. **Part 10 is a different project** in a different repo with no owner. The plan is blocked on it. If it doesn't ship, the plan can't ship. If it ships differently than specified here, assumptions break.

2. **Part 10.5 (metadata filtering)** is scope creep — SQLite already does this. Drop it from the plan or justify why the librarian needs to duplicate SQLite's structured filter capability.

3. **`ChunkByHeadings` + `TextChunk` move** is unnecessary post-removal. Skip Step 16b; just delete the dead code with the package.

4. **Backfill** creates version churn for already-synced pages. Add an idempotency note or accept the noise.

5. **Second delete path** (`api/pages.go:212`) is a pre-existing orphan source the plan ignores.

**Bottom line:** The plan achieves its stated goal (remove local vectordb, use librarian for search) if and only if Part 10 ships in pippi-librarian. That's a management/scheduling dependency, not a technical flaw. The prd2wiki technical work is correct.

---

## Step 11 — Related files

| Path | Role |
|------|------|
| `docs/plan-remove-local-vectordb.md` | Canonical plan — **not modified** |
| `docs/plan-remove-local-vectordb-REVIEW-7-big-picture.md` | **This file** |

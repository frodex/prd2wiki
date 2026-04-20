# Deep review: `plan-remove-local-vectordb.md` (second pass)

**Scope:** Review notes only — do **not** edit the plan file unless the plan owner requests it.

## Codebase under review (read this first)

All file paths, greps, and behavioral claims in this document refer to **one** tree only:

| Field | Value |
|--------|--------|
| **Root path** | `/srv/prd2wiki` |
| **Remote** | `https://github.com/frodex/prd2wiki.git` (`origin`) |
| **Branch** | `main` |
| **Commit** | `b522b6a0f7d4d984077e9e031955d2ab3f6c8423` (pin this when comparing reviews; re-run `git rev-parse HEAD` after pulls) |

**Explicitly out of scope for this review** (not opened, not diffed, not run here):

- **pippi-librarian** — separate service/repo; socket path in config is referenced but its implementation was not verified.
- **`prd2wiki-import` or any import CLI** — asserted absent **in this repo only**; if it exists in another repository or branch, that was not examined.
- **Sibling directories** under `/srv/` or other workspaces — not compared.
- **Runtime behavior** — no servers were started; conclusions are from **static** reading of the commit above.

**In-repo design docs** (e.g. under `docs/wiki-local/`) were cited only as **documentation** about planned tools; they are not treated as implementation unless the same behavior appears in Go code under `/srv/prd2wiki`.

### Related documents (full paths)

All paths are under the **prd2wiki** repo root (`/srv/prd2wiki` on this machine).

| File | Role |
|------|------|
| `/srv/prd2wiki/docs/plan-remove-local-vectordb.md` | **Canonical plan** — read-only for reviewers; may include **“Codebase scope (required)”** and `app.go` **`New()`** anchors (verify against current `main`). |
| `/srv/prd2wiki/docs/plan-remove-local-vectordb-RESPONSE-verification.md` | Earlier pass: code verification vs the plan (first-layer source check). |
| `/srv/prd2wiki/docs/plan-remove-local-vectordb-REVIEW-2-deep.md` | **This file** — second-pass deep review and assumptions check. |

If you only read one review artifact, use **this** file for scope + blockers; use **RESPONSE-verification** for the inventory-by-inventory table.

---

Repo verification only (no running services). Cross-checked against the tree at the commit in the table above.

---

## 1. Confirmed blockers / doc bugs

### 1.1 `prd2wiki-import` does not exist in this repo

The plan Part 6 says bulk backfill is “what **prd2wiki-import** already does (Phase 5).”

- **Fact:** There is **no** `cmd/prd2wiki-import` or `prd2wiki-import` binary in the workspace (`Glob` returns 0).
- **Inference:** That sentence is **aspirational** (matches wiki-local design docs like `cec9acb.md`) or refers to a **different repo**. It must **not** be cited as “already implemented” here.
- **Fix:** Rephrase to “implement or run a one-off backfill (same contract as future import tool)” or point to the **actual** script/binary once it exists.

### 1.2 Web search ≠ API search (behavioral split)

| Surface | Behavior (verified in code) |
|--------|-----------------------------|
| **HTTP API** `internal/api/search.go` | Runs **FTS and `lib.Search` in parallel**, then **merges** (SQL rows first, then vector IDs not already seen). |
| **Web UI** `internal/web/search.go` | Calls **`lib.Search` first**; **only if `len(items)==0`** does it fall back to FTS. |

So: **not** “the same merge logic” as the API. Any plan wording that implies one unified search path for **both** is **wrong** unless you add a follow-up change to align web with API (or document the intentional UX difference).

### 1.3 Deleting `internal/vectordb/` is not “delete folder + tests”

`internal/librarian/normalizer.go` imports **`vectordb.TextChunk`** for `ChunkByHeadings`. Removing the package requires **moving `TextChunk`** (or an equivalent struct) into `librarian` or a neutral package **before** deleting `vectordb`.

### 1.4 Constructor name in inventory

`internal/app/app.go` exports **`New(...)`**, not `Run`. If the plan’s inventory table still says `Run()`, correct it.

---

## 2. Scoring / parity (avoid false “equivalence”)

### 2.1 Local `vectordb.Store.Search` is not “pure cosine”

`internal/vectordb/store.go` `Search` uses **fused** scoring: `0.7*cosine + 0.3*keywordScore(query, r.Text)` on in-memory records.

**Librarian** search goes through **pippi** hybrid (BM25 + vector + RRF, per prior verification). After migration, **ranking will change**; call it **intended** or **acceptable**, not “identical behavior.”

### 2.2 Query path: vocabulary normalization

`Librarian.Search` normalizes query tokens via **`l.vocab.Normalize`** before calling the store. Whether **memory_search** applies the same normalization is **out of band** for this repo alone — flag as **parity assumption** if product cares.

---

## 3. Dead code / cleanup (plan alignment)

### 3.1 `DedupDetector` / `flags.dedup` in `submit()`

`internal/librarian/librarian.go` `submit()` still constructs **`DedupDetector`** and checks **`flags.dedup`**, but **`flags` is never populated** from options in that path — effectively **dead**. Safe to remove with the plan’s “delete dedup” phase; **watch** `librarian_test.go` for tests that assert dedup warnings.

### 3.2 `FindSimilar`

Exposed on **`Librarian`** and implemented in **`vectordb.Store`**, but **no** callers in `internal/api` or `internal/web` were found. Removing local store means **either** drop the method **or** reimplement via librarian/MCP if product wants it later.

### 3.3 Steward

`grep` over `internal/steward` shows **no** `vectordb` / `vstore` usage — local vector removal does **not** appear blocked by steward MCP in this tree.

---

## 4. Config and binaries

`config/prd2wiki.yaml` still has an **`embedder:`** block (TEI endpoint comment). After removing local embedding from prd2wiki, confirm **every** binary that loads this config — if embedder is **only** for local vectordb, YAML + docs can be trimmed; if something else still reads it, keep or rename.

---

## 5. `RemoveFromIndexes` and tree API

`RemoveFromIndexes` is used from **`internal/api/tree_api.go`** (delete flow). The plan should keep **“remove from remote/librarian index”** in scope when local `vstore.RemovePage` disappears — i.e. **memory_delete** or equivalent must be wired **before** local removal is deleted, or deletes will **orphan** remote memory.

---

## 6. Assumptions double-checked

| Assumption | Status |
|------------|--------|
| API merges FTS + vector | **Verified** (`internal/api/search.go`). |
| Web uses vector-first, FTS only when empty | **Verified** (`internal/web/search.go`). |
| `libclient.New` returns `(client, err)` | **Verified** — startup can continue with `client == nil`. |
| `prd2wiki-import` exists here | **False** — design-only in wiki-local docs. |
| Steward depends on local vectordb | **No matches** in `internal/steward`. |
| `TextChunk` ties normalizer to vectordb | **Verified** — refactor required to delete package. |

---

## 7. Suggested plan edits (minimal)

1. Part 6: Replace “prd2wiki-import already does” with **accurate** wording (script TBD / other repo / future binary).
2. Add explicit **web vs API search** subsection — **BLOCKER** for any “single search implementation” claim.
3. Step “delete `internal/vectordb`”: add **move `TextChunk`** (or duplicate type) **first**.
4. Inventory: **`New`** not `Run`; optional: note **local Search fusion** vs librarian hybrid for expectations.
5. Part 8 / gates: **tree delete → memory_delete** ordering as a merge **blocker** next to bulk backfill.

---

*Generated for traceability; supersede if the plan or codebase changes.*

# Stepped review (dual-repo): `plan-remove-local-vectordb.md` — revision after Review 5

**Procedure:** New review file only. **Does not modify** the plan or prior review artifacts.

---

## Step 0 — Scope (required)

| Field | Value |
|--------|--------|
| **Plan under review** | `docs/plan-remove-local-vectordb.md` (workspace copy; incorporates Review 5 in header) |
| **prd2wiki root** | `/srv/prd2wiki` |
| **prd2wiki commit** | `54c18447b765847a49da33aa6188a44d09a09c07` |
| **pippi-librarian root** | `/srv/pippi-librarian` |
| **pippi-librarian commit** | `cc96c4bd10aa6b0cec0669ae9bf6c795dd3e9291` |
| **Method** | Static read of cited Go files in both trees |

---

## Step 1 — What changed in the plan (vs Review 5)

| Topic | Review 5 finding | Current plan |
|-------|------------------|--------------|
| **Part 5 vs Step 4** | Option B vs wiki prepend conflict | **Step 4 removed**; wiki does **not** prepend; Part 10 owns enrichment — **resolved in plan text** |
| **`memory_delete` API** | Plan implied page_uuid | **Step 11 + 11b** document **`id` = mem_**, options A/B/C — **aligned with librarian** |
| **Review 5 items** | Listed in plan header | **Incorporated** |

---

## Step 2 — prd2wiki verification (inventory + delete path)

| Plan claim | Code check (`prd2wiki` @ Step 0 commit) | Result |
|------------|------------------------------------------|--------|
| Local pipeline in `app.go` `New()`, `librarian` + `vectordb`, API/web search split | Unchanged from prior reviews; files `internal/app/app.go`, `internal/api/search.go`, `internal/web/search.go`, `internal/librarian/librarian.go`, `internal/vectordb/store.go` | **Still accurate** |
| **`RemoveFromIndexes(pageID)`** from **`tree_api.go`** with **page UUID** | `internal/api/tree_api.go`: `lib.RemoveFromIndexes(ent.Page.UUID)` | **Accurate** |
| **Option A: mem_ from `.link` line 2** | `internal/tree/scanner.go` `parseLinkFile`: line 2 → **`Page.LibrarianID`**; `PageEntry` exposes **`Page.LibrarianID`** via `Index.PageByUUID` | **Feasible without parsing files by hand** — use **`treeHolder.Get().PageByUUID(pageUUID).Page.LibrarianID`** if tree is current |
| **Empty line 2** | If **`LibrarianID` == ""** (never synced), Option A yields **no mem_ id** — plan should fall through to **Option B** (`memory_get` by page_uuid) or skip delete with warn | **Gap** — plan mentions Option A as recommended but not **empty-head** behavior |

---

## Step 3 — pippi-librarian verification (Part 5 + Part 10 + Step 11)

| Plan claim | Code check (`pippi-librarian` @ `cc96c4b`) | Result |
|------------|---------------------------------------------|--------|
| **`memory_store` drops metadata** | `cmd/pippi-librarian/main.go`: `StoreWiki(ctx, ns, pageUUID, content)` only | **Accurate** |
| **`StoreWiki`** arity | `internal/librarian/memory.go`: `(ctx, namespace, pageUUID, content string)` | **Accurate** |
| **Part 5 bullet: “LanceDB Arrow schema has no ext_json…”** | `internal/librarian/memory_lance.go` `buildMemoryLanceSchema` includes **`meta_json`** (not `ext_json`) | **Imprecise wording** — column **`meta_json` exists**; wiki metadata is not wired into it for wiki rows today, but “no ext_json” is misleading. Part **10.3** proposes adding **`ext_json`** — implementers should **reconcile with existing `meta_json`** to avoid duplicate JSON columns |
| **`memory_delete` requires `id` (mem_)** | `main.go` + `schema.d/wiki.yaml` | **Accurate**; matches updated Steps **11 / 11b** |
| **`SearchWiki` title = `firstLineTitle(rec.Content)`** | `internal/librarian/memory.go` | **Accurate**; Part **10.3b** correctly calls this out |

---

## Step 4 — Cross-repo: Step 2 “double normalization”

- **prd2wiki** `Librarian.Search`: tokenizes, applies **`l.vocab.Normalize`**, rejoins.
- **pippi** `Table.SearchAmong`: **`NormalizeSemantic(query)`** on the **whole query string** for embedding.

Plan Step 2 states wiki-normalized query is then normalized again in the librarian — **different tokenization shape** (word-split wiki vs full-string semantic in pippi). Treat as **behavior change risk**, not “harmless” in all languages/inputs. **Acceptable** if product signs off; not bitwise parity.

---

## Step 5 — Checklist hygiene (plan Part 9)

**Part 9 item 2** still says verify sync sends “**content (with title prefix per Part 5)**.” After the revision, **Part 5** says **no wiki prepend** until librarian stores metadata (**Option B + Part 10**). That checklist line is **stale** relative to the updated Part 5 / Step 4 removal — reviewers should verify **metadata in `ext` / `MemoryStore` args**, not “prefixed content,” unless the checklist is updated in a future plan edit.

---

## Step 6 — Cosmetic (non-blocking)

- Inventory rows **#19** and **#20** remain **out of numeric order** in the markdown table (lines ~88–89).

---

## Step 7 — Verdict

| # | Conclusion |
|---|------------|
| 1 | The **updated** plan **fixes** the major Review **5** contradictions (**Step 4 vs Part 5**, **`memory_delete` contract**). |
| 2 | **Part 5 / Part 10** should **name `meta_json`** (existing) vs inventing **`ext_json`** without migration strategy — **clarify in implementation**, not assumed here. |
| 3 | **Option A** for delete should document **`LibrarianID` empty** fallback (B or no-op + log). |
| 4 | **Part 9 checklist item 2** should track **metadata**, not **title-prefixed body**, to match current Part 5. |
| 5 | Core **prd2wiki** + **pippi** facts used in the plan remain **verifiable** at the pinned commits. |

---

## Step 8 — Related files

| Path | Role |
|------|------|
| `docs/plan-remove-local-vectordb.md` | Canonical plan — **not modified** by this review |
| `docs/plan-remove-local-vectordb-REVIEW-6-stepped-dual-repo.md` | **This file** |

---

*Re-pin commits in Step 0 after `git pull`; re-verify Part 10 when pippi moves past `cc96c4b`.*

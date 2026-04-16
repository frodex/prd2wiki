# Fossil skin layering (twoWiki / prd2wiki vendor)

Agent-facing wiki detail can also live on prd2wiki; this file is the **checkout-local** map of what `apply_twowiki_skin.py` merges.

New **Lovable drops** (e.g. `one-line-menu-ticket-tags-02a/`) must be **merged**, not copied over the repo wholesale — see **`MERGE-DROPS.md`**.

## Merge order (CSS, single `config.css` value)

Pieces are concatenated **top to bottom**. Later rules win on specificity ties.

| Layer | Source | Role |
|-------|--------|------|
| 1 | `lovable_01a/css.txt` | Base chrome from the Lovable export (layout primitives). **Prefer not to hand-edit** — regenerate from export when possible. |
| 2 | `twowiki-fossil-th1-append.css` | **Structural / compatibility** fixes (float resets, ticket column width, Mermaid overflow, setup tweaks). Small, reviewable edits. |
| 3 | `one-line-menu-ticket-tags-01a/twowiki-fossil-skin-v6.css` | **Style / palette / typography** (single-line header, doc tables, pills). Appended **last** so design wins. |
| 3b (optional) | `one-line-menu-ticket-tags-02a/css.txt` (as `twowiki-fossil-skin-v7.css` after review) | Next design drop — **replace or follow** layer 3 only after diff + merge; not wired until `apply_twowiki_skin.py` is updated deliberately. |

**Do not** fix ticket **behavior** in layer 3 only — if JS or CSP is required, change `ticket-viewpage.th1`, `footer.th1`, or `default-csp` in the apply script.

### Optional convention: “do not edit above this line”

For hand-maintained CSS layers (2 and 3), teams sometimes add:

```text
/* === APPENDED PATCHES BELOW — do not reorder === */
```

at a stable anchor so automation or humans only **append** below that banner. Not enforced by tooling today; document in PR when you introduce it.

## Non-CSS config rows (all required for “twoWiki ticket reader” baseline)

| `config.name` | File | MUST for baseline |
|----------------|------|---------------------|
| `ticket-viewpage` | `ticket-viewpage.th1` | **Yes** — `.twowiki-doc`, markdown body, **task lists + sortable tables** (classic script; not inside Mermaid `type=module`). |
| `footer` | `footer.th1` | **Yes** — site-wide Mermaid (non-tktview), `/ticket/HASH` → `/tktview`, Setup links. |
| `ticket-editpage` | `ticket-editpage.th1` | **Yes** for edited ticket UX parity (keep in sync with view assumptions where applicable). |
| `default-csp` | inline in `apply_twowiki_skin.py` | **Yes** if using Mermaid/ELK from CDN (`wasm-unsafe-eval`, `blob:`, jsdelivr). |
| `css` | merged layers above | **Yes** — without append + v6, stock floats / width break the ticket column. |
| `header`, `details`, `js` | `lovable_01a/*` | **Yes** for chrome parity with the chosen package. |
| `default-skin` | SQL in apply script | **Yes** — set to `custom` so CONFIG skin is used. |

## Fossil markdown tables (tickets)

Pipe tables in tickets must follow **Fossil’s** rules, not GitHub’s. A `|-----|-------|` “GFM delimiter” row is parsed as **data**, which splits the matrix into many broken `md-table` blocks. See **`FOSSIL-TICKET-MARKDOWN-TABLES.md`** for the correct separator line (dash-only horizontal rule under the header).

## Sortable tables (behavior contract)

- Fossil markdown tables are emitted with class **`md-table`**. The qualifying matrix fixture is **many one-row `md-table` blocks**; sorting is still attached per table (Fossil output shape), and header clicks reorder that table’s single body row relative to separators (no-op but still “works” for the test harness).
- Raw **HTML** `<table>` in a ticket body is still forced through markdown and often lands in `<pre>`; those tables are **not** `md-table`. For those, sort hooks apply only when there are **≥ 2** non-separator data rows, or when the table has class **`twowiki-table-sort`** (explicit opt-in).

## Applying

```bash
python3 vendor/twowiki-fossil-skin/apply_twowiki_skin.py | fossil sql -R /path/to/repo.fossil
```

Lab helper: `scripts/apply-twowiki-skin-lab.sh`.

After apply, hard-refresh the browser (cached `/style.css?id=…`).

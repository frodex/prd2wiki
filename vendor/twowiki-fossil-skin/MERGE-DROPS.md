# Merging a new skin drop (e.g. `one-line-menu-ticket-tags-02a/`)

## Principle: do not put “breaks the site if missing” in overwritable files

Some files exist to be **refreshed or replaced wholesale** when design ships a new drop (e.g. **`…/twowiki-fossil-skin-v7.css`**, or the **pure palette/layout** parts of `lovable_01a/css.txt`). Treat those as **presentation-only**.

**Do not** rely on those files alone for anything that **breaks the site** if overwritten: Mermaid/ELK wiring, ticket redirects, `rpage-rptview` behavior, CSP, markdown/ticket TH1, sortable-table hooks, float resets against Fossil `default.css`, etc.

Put **behavior-critical** material in **dedicated, intentionally merged** files that are **not** meant to be replaced by a Lovable export:

| Keep behavior here | Why |
|--------------------|-----|
| `footer.th1` | Scripts, redirects, site-wide Mermaid gate. |
| `ticket-viewpage.th1` / `ticket-editpage.th1` | Ticket rendering, markdown helpers, sort JS. |
| `twowiki-fossil-th1-append.css` | Structural / compatibility CSS that must survive a design refresh (or duplicate tiny “must keep” rules here if they were only in an old v6 tail). |
| `apply_twowiki_skin.py` | `default-csp` and merge order — not in a designer CSS file. |

If a rule is **required** for correctness and might be lost when someone drops in a new v8 CSS file, **move it** (or copy it) into **`twowiki-fossil-th1-append.css`** (or the appropriate TH1), not only into the volatile design layer.

---

Exports from design tools are **full pages** (DOCTYPE + `header` + `footer` that close the document). The twoWiki stack is **split** on purpose:

| Piece | Source of truth today |
|-------|------------------------|
| Merged **`config.css`** | `lovable_01a/css.txt` + `twowiki-fossil-th1-append.css` + design layer (`01a` v6 CSS, soon maybe `02a` v7). |
| **`config.header`** | `lovable_01a/header.txt` — **partial** HTML + **TH1** (`$mainmenu`, `hbmenu.js`, guest/login). |
| **`config.footer`** | `footer.th1` — Mermaid/ELK, ticket redirect, report-view JS, Setup links — **not** Lovable `footer.txt`. |
| **Tickets** | `ticket-viewpage.th1` / `ticket-editpage.th1` — behavior + markdown; not in a pure CSS drop. |

**Do not** copy `02a/header.txt` or `02a/footer.txt` over those files without merging.

## Better workflow than “replace and pray”

1. **Treat the drop as read-only input** — keep `02a/` unchanged; merged results live in `lovable_01a/` and/or a **single new design file** (e.g. `02a/twowiki-fossil-skin-v7.css` cut from `css.txt`).

2. **CSS (colors/layout)**  
   - **Diff** `02a/css.txt` against `one-line-menu-ticket-tags-01a/twowiki-fossil-skin-v6.css` (or against the concatenation you care about).  
   - **Last layer wins:** append v7 **after** `twowiki-fossil-th1-append.css` in `apply_twowiki_skin.py`, or fold v7 rules into one reviewed file.  
   - **Re-append** any small **repo-only** rules that must not disappear (sortable thead hover, etc.) — either at the end of v7 or in `twowiki-fossil-th1-append.css`.

3. **Header**  
   - Compare **inner** markup (inside `<header>` … breadcrumb) to `lovable_01a/header.txt`.  
   - Preserve **TH1** (`foreach $mainmenu`, `builtin_request_js hbmenu.js`, login fallback).  
   - Do **not** paste `$<menu.timeline.class>` literals — they do not expand in CONFIG header; the loop does.

4. **Footer**  
   - Ignore Lovable `footer.txt` for **behavior**. If you need a string change, edit **`footer.th1`** and keep scripts.

5. **Optional tooling** (future): a small script `merge_skin_review.py` that prints file sizes + `diff -u` summaries and refuses to write unless `--force` — still **human** merge for TH1.

6. **Apply**  
   - After merge, run `apply_twowiki_skin.py` once; confirm `config` lengths; hard-refresh `/style.css`.

## Why not one zip = one `fossil config`?

Fossil stores **one blob per key**; layering is **our** convention (concat CSS, separate TH1 files). Vendor zips are **not** shaped for that unless we split and merge every time.

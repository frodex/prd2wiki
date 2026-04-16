# twoWiki — Fossil skin overlay (prd2wiki-like ticket reader)

This folder contains **repository `config` payloads** for Fossil 2.29+ to make `/tktview` look closer to prd2wiki: centered column, **header + `nav.mainmenu`**, compact badges, prose styling for markdown, optional Mermaid, and a footer link back to prd2wiki.

**How CSS is layered (base → structure → design):** read **`SKIN-LAYERING.md`**. `apply_twowiki_skin.py` concatenates `lovable_01a/css.txt` + `twowiki-fossil-th1-append.css` + `one-line-menu-ticket-tags-01a/twowiki-fossil-skin-v6.css`.

**Institutional / agent docs:** Behavior, TH1 techniques, verification, and related wiki links live on the **prd2wiki** page **[twoWiki Fossil skin — implementation notes (agents)](http://192.168.22.56:8082/prd2wiki/twowiki-fossil-skin-implementation-notes)** (public: `https://wiki.droidware.ai/prd2wiki/twowiki-fossil-skin-implementation-notes`). This README only tracks **file layout**, **apply commands**, and **markdown/TH1 pipeline reminders** for developers with a checkout.

## Files

| File | Fossil `config.name` | Purpose |
|------|----------------------|---------|
| `lovable_01a/css.txt` | (part of `css`) | Base skin CSS from the Lovable export (first layer in merged `css`). |
| `one-line-menu-ticket-tags-01a/twowiki-fossil-skin-v6.css` | (part of `css`) | Design layer (last in merge). Single-line header, v6 doc/ticket chrome. |
| `lovable_01a/header.txt` | `header` | Chrome: logo area, title, `nav.mainmenu` + hamburger (`hbmenu.js`). |
| `lovable_01a/details.txt` | `details` | Skin details (pikchr / graph toggles). |
| `lovable_01a/js.txt` | `js` | Optional skin JS (package ships a comment-only placeholder). |
| `twowiki-fossil-th1-append.css` | (part of `css`) | Appended after package CSS: float resets, `.sectionmenu`, ticket column width, Mermaid overflow, setup-page tweaks. |
| `ticket-viewpage.th1` | `ticket-viewpage` | Ticket view: `.twowiki-doc`, sortable tables + task lists (classic script). |
| `ticket-editpage.th1` | `ticket-editpage` | Ticket editor TH1. |
| `footer.th1` | `footer` | Mermaid 11 + ELK (module), ticket `/ticket/HASH` redirect, Setup/skin links — **not** `lovable_01a/footer.txt`. |
| `apply_twowiki_skin.py` | — | Emits SQL for `css`, `header`, `details`, `js`, **`mainmenu`** (stock bar + **Tickets → `/rptview/1`**), tickets, `footer`, `default-csp`. Re-run apply after editing Admin **Configuration** if you want this file to remain source of truth. |

**Mermaid / ELK** stay in **`footer.th1`** (runs after content). **`header`** is the Lovable layout only; do not move Mermaid there without revisiting CSP and load order.

**Tickets vs repo “file tree”:** Fossil does not attach a browsable tree to a ticket record. **`ticket-viewpage.th1`** adds links to **`/dir?ci=tip`** (file list) and **`/tree?ci=tip`** (tree) plus **Timeline**; **`page_path`** (if present) still shows as the path badge. Deeper “this ticket ↔ these commits/files” needs ticket fields or JSON tooling outside this skin.

**Fossil ticket pipe tables:** Not the same as GitHub GFM. A `|-----|-------|` row is **data**, not a delimiter, and splits the matrix into many one-row tables. See **`FOSSIL-TICKET-MARKDOWN-TABLES.md`**.

## Markdown vs wiki (important)

- TH1 **`wiki`** renders [Fossil wiki](https://fossil-scm.org/home/doc/trunk/www/wiki.wiki) markup — **not** CommonMark and **not** the same as prd2wiki’s **Goldmark** pipeline.
- TH1 **`markdown`** runs Fossil’s `markdown_to_html()` (fenced code with `language-*` classes, tables, etc.) — this is what you want for bodies imported from prd2wiki. Mermaid fenced blocks must use ` ```mermaid ` so the HTML becomes `pre > code.language-mermaid`.

The ticket template in this folder uses **`markdown`** + **`untaint`** + **`lindex $mdout 1`** (second list element is HTML body; first is extracted title).

**Line breaks:** CommonMark treats a single newline inside a paragraph as a space, so raw textarea lines would collapse to one line in HTML. The skin defines **`twowiki_md_hardbreaks`** (TH1-safe: no `regsub` / no `while` / no `append` — uses `for` + `string index` + `set "${var}…"`): outside fenced ``` blocks it inserts Markdown hard breaks (two spaces before each newline) so multi-line prose and remarks keep their line structure; fenced code blocks are copied verbatim.

**Sortable tables + task lists:** Implemented in a **classic** `<script>` in **`ticket-viewpage.th1`** (not inside the Mermaid **`type="module"`** block). If the Mermaid CDN ESM import fails, table sorting and GFM task-list checkboxes still run. Sorting supports **`<thead>`/`<th>`** and **header row as first `<tbody>` row** when `<thead>` is omitted.

**Sortable rules:** Fossil markdown emits **`md-table`**; **one-row `md-table` blocks** (qualifying / matrix fixtures) **keep** click-to-sort on each table. Separator-like `tbody` rows (`|-----|` style, all-dash cells) are skipped for ordering. **Non-`md-table`** HTML tables need **≥ 2** real data rows **or** class **`twowiki-table-sort`** on `<table>` to get sort hooks (avoids junk raw-HTML shells). Raw HTML in J fields still tends to render inside `<pre>` — that is Fossil/markdown behavior, not this skin’s sort JS.

## Apply (twowiki host)

```bash
python3 apply_twowiki_skin.py | fossil-json sql -R /opt/twowiki/repo.fossil
```

**If the site looks unchanged after editing this repo:** the live `.fossil` still has the old `config` rows until you run the command above (or `scripts/apply-twowiki-skin-lab.sh`). Then hard-refresh `/style.css` in the browser (cache). To confirm what is deployed: `fossil sql -R repo.fossil "SELECT length(value) FROM config WHERE name='css';"`.

**Skin selection (Fossil):** lower rank wins. The **`skin=`** query param and **`fossil_display_settings`** cookie (**rank 2**) beat **`default-skin`** (**rank 3**) and **CONFIG css/header/footer** (**rank 4**). If a browser still shows a built-in look, open **`/skins?skin=custom`** once (footer has a **Site skin** link) to set the display cookie to the repository’s CONFIG skin. **`apply_twowiki_skin.py`** sets **`default-skin`** to the literal **`custom`** (not a built-in name) so the server default path does not pick **`plain_gray`** over your CONFIG rows.

**Skin / Admin UI:** With only `css` + `footer` in `config`, stock Admin links can be awkward. Logged-in **Setup or Admin** users get a yellow **Setup** strip at the bottom with direct links to **`/setup_skinedit`** (edit CSS/TH1) and **`/setup_skin`**. If setup form controls still do not respond to clicks, try again after deploy — `twowiki-fossil-th1-append.css` relaxes `nav.mainmenu` stacking on `body[class*="setup_"]` pages so the sticky bar does not sit above Fossil’s controls.

Also set display name (once):

```sql
INSERT OR REPLACE INTO config(name,value,mtime)
VALUES('project-name','twoWiki',julianday('now'));
```

## CSP

`default-csp` allows `https://cdn.jsdelivr.net` and **`blob:`** in `script-src`, plus **`'wasm-unsafe-eval'`** on `script-src` / `worker-src`, and **`worker-src`** includes jsdelivr — needed for **Mermaid 11 + ELK** (elkjs WebAssembly and workers). Stock skins often omit `default-csp`, so ELK can work on a built-in skin but fail on the custom skin until this row is applied. The `$nonce` token is expanded by Fossil when emitting the CSP header. **Cloudflare** (or another proxy) may add a **second** CSP header that still blocks `wasm-unsafe-eval` or jsdelivr; check the **effective** policy in DevTools → Network → document → Response headers, and the console for CSP violations.

## Reset

```sql
DELETE FROM config WHERE name IN ('css','header','footer','details','js','mainmenu','default-csp');
DELETE FROM config WHERE name='ticket-viewpage';
-- optional: DELETE FROM config WHERE name='ticket-editpage';
-- optional: DELETE FROM config WHERE name='project-name';
```

Fossil falls back to compiled-in defaults when rows are absent.

## See also

- **[twoWiki Fossil skin — implementation notes (agents)](http://192.168.22.56:8082/prd2wiki/twowiki-fossil-skin-implementation-notes)** — canonical institutional doc (remote agents: start here).
- [Fossil Instance — Configuration and Setup](http://192.168.22.56:8082/projects/default/pages/be72994) — JSON ticket API, schema, original `ticket-viewpage` notes.
- [PLAN: twoWiki — Fossil ticket view parity with prd2wiki (skin & UX)](http://192.168.22.56:8082/projects/default/pages/eb5acde2-3f7a-48d2-bab3-53be54ee44f2)

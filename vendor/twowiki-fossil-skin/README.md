# twoWiki — Fossil skin overlay (prd2wiki-like ticket reader)

This folder contains **repository `config` payloads** for Fossil 2.29+ to make `/tktview` look closer to prd2wiki: centered column, **header + `nav.mainmenu` styled like prd2wiki `.wiki-nav`** (`#1a1a2e`, system sans-serif, no underlines on top nav links), compact badges, prose styling for markdown, optional Mermaid, and a footer link back to prd2wiki.

## Files

| File | Fossil `config.name` | Purpose |
|------|----------------------|---------|
| `twowiki-fossil.css` | `css` | Appended skin CSS (scoped to `body.tkt` and `.twowiki-*`). |
| `ticket-viewpage.th1` | `ticket-viewpage` | Ticket view TH1: compact header + `.twowiki-doc` wrappers. |
| `footer.th1` | `footer` | Mermaid loader (jsdelivr) + nonce’d init script + prd2wiki link. |
| `apply_twowiki_skin.py` | — | Prints SQL `INSERT OR REPLACE INTO config` for the above plus `default-csp`. |

## Markdown vs wiki (important)

- TH1 **`wiki`** renders [Fossil wiki](https://fossil-scm.org/home/doc/trunk/www/wiki.wiki) markup — **not** CommonMark and **not** the same as prd2wiki’s **Goldmark** pipeline.
- TH1 **`markdown`** runs Fossil’s `markdown_to_html()` (fenced code with `language-*` classes, tables, etc.) — this is what you want for bodies imported from prd2wiki. Mermaid fenced blocks must use ` ```mermaid ` so the HTML becomes `pre > code.language-mermaid`.

The ticket template in this folder uses **`markdown`** + **`untaint`** + **`lindex $mdout 1`** (second list element is HTML body; first is extracted title).

**Line breaks:** CommonMark treats a single newline inside a paragraph as a space, so raw textarea lines would collapse to one line in HTML. The skin defines **`twowiki_md_hardbreaks`** (TH1-safe: no `regsub` / no `while` / no `append` — uses `for` + `string index` + `set "${var}…"`): outside fenced ``` blocks it inserts Markdown hard breaks (two spaces before each newline) so multi-line prose and remarks keep their line structure; fenced code blocks are copied verbatim.

## Apply (twowiki host)

```bash
python3 apply_twowiki_skin.py | fossil-json sql -R /opt/twowiki/repo.fossil
```

Also set display name (once):

```sql
INSERT OR REPLACE INTO config(name,value,mtime)
VALUES('project-name','twoWiki',julianday('now'));
```

## CSP

`default-csp` allows `https://cdn.jsdelivr.net` in `script-src` alongside `'nonce-$nonce'` so Mermaid can load. The `$nonce` token is expanded by Fossil when emitting the CSP header.

## Reset

```sql
DELETE FROM config WHERE name IN ('css','footer','default-csp');
DELETE FROM config WHERE name='ticket-viewpage';
-- optional: DELETE FROM config WHERE name='project-name';
```

Fossil falls back to compiled-in defaults when rows are absent.

## See also

- [Fossil Instance — Configuration and Setup](http://192.168.22.56:8082/projects/default/pages/be72994) — JSON ticket API, schema, original `ticket-viewpage` notes.
- [PLAN: twoWiki — Fossil ticket view parity with prd2wiki (skin & UX)](http://192.168.22.56:8082/projects/default/pages/eb5acde2-3f7a-48d2-bab3-53be54ee44f2)

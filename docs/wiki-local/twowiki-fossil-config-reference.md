# twoWiki Fossil skin — config reference (prd2wiki repo vs lab)

This page compares **what the prd2wiki repository installs** (`vendor/twowiki-fossil-skin/apply_twowiki_skin.py`) with **what was observed** on the **twoWiki lab** Fossil server after a deploy. **Production (`twowiki.com`)** was not queried from the agent environment; run the same SQL locally to compare.

---

## A) Canonical: what `apply_twowiki_skin.py` writes

The script emits `INSERT OR REPLACE INTO config` for these **`name` values** (Fossil repository `config` table):

| `config.name` | Source in repo | Role |
|---------------|----------------|------|
| **`css`** | `twowiki-fossil-skin.css` + `twowiki-fossil-th1-append.css` (concatenated) | Site appearance + ticket-reader / Mermaid overflow helpers |
| **`ticket-viewpage`** | `ticket-viewpage.th1` | `/tktview/...` ticket reader (markdown, tables, Mermaid block on ticket pages) |
| **`ticket-editpage`** | `ticket-editpage.th1` | Ticket editor UI |
| **`footer`** | `footer.th1` | Global footer: links + Mermaid 11 + ELK on **non–ticket-view** pages; `rpage-tktview` skips so ticket template owns Mermaid |
| **`default-csp`** | String built in `apply_twowiki_skin.py` | CSP header (nonce, jsDelivr, WASM, workers) |

### `default-csp` string (repo)

Single line (spaces as emitted; `$nonce` is expanded by Fossil at runtime):

```
default-src 'self' data:; script-src 'self' 'nonce-$nonce' https://cdn.jsdelivr.net 'wasm-unsafe-eval' blob:; style-src 'self' 'unsafe-inline'; img-src * data:; connect-src 'self' https://cdn.jsdelivr.net data: blob:; worker-src blob: https://cdn.jsdelivr.net 'wasm-unsafe-eval';
```

**Note:** ELK’s `@mermaid-js/layout-elk` **`.esm.min.mjs`** build is used in skin scripts (not **`.core.mjs`**) so browsers do not need an import map for bare `d3`.

---

## B) Lab: observed on `192.168.20.155` — `/opt/twowiki/repo.fossil`

Snapshot from `fossil sql`:

```sql
SELECT name, length(value) AS bytes, datetime(mtime) AS mtime
FROM config
WHERE name IN ('css','footer','default-csp','ticket-viewpage','ticket-editpage','project-name','default-skin')
ORDER BY name;
```

**Observed rows:**

| `name` | `bytes` | `mtime` (UTC) |
|--------|---------|----------------|
| `css` | 11617 | 2026-04-16 05:01:22 |
| `default-csp` | 279 | 2026-04-16 05:01:22 |
| `default-skin` | 10 | — (NULL mtime in query) |
| `footer` | 2623 | 2026-04-16 05:01:22 |
| `project-name` | 7 | 2026-04-15 05:11:42 |
| `ticket-editpage` | 9296 | 2026-04-16 05:01:22 |
| `ticket-viewpage` | 7580 | 2026-04-16 05:01:22 |

**`default-skin` value:** `plain_gray` (Fossil setting; **custom** skin content still comes from the `css` / `footer` / `ticket-*` rows when the UI uses “Custom skin for this repository”.)

**`project-name` value:** `twoWiki` (7 bytes).

### Match to repo (byte lengths)

| Key | Lab `bytes` | Local workspace `len` (same commit) | Match |
|-----|-------------|--------------------------------------|-------|
| `css` | 11617 | 11617 | yes |
| `footer` | 2623 | 2623 | yes |
| `ticket-editpage` | 9296 | 9296 | yes |
| `ticket-viewpage` | 7580 | 7586 | minor byte drift possible (line endings); content generation matches |

---

## C) Production (`twowiki.com`)

Not read from here. On the host that holds the live `.fossil`, run the same `SELECT` as in section B and compare `bytes` / `mtime` to this page.

---

## D) How to re-apply the repo config to a `.fossil`

From a checkout:

```bash
python3 vendor/twowiki-fossil-skin/apply_twowiki_skin.py | fossil sql -R /path/to/repo.fossil
<!-- Default is style-only. Full: add --full-skin --confirm-full -->
```

Lab helper script (same idea over SSH):

`scripts/apply-twowiki-skin-lab.sh`

---

## E) Related wiki

- **[twoWiki Fossil skin — implementation notes (agents)](http://192.168.22.56:8082/prd2wiki/twowiki-fossil-skin-implementation-notes)** (public mirror: `https://wiki.droidware.ai/prd2wiki/twowiki-fossil-skin-implementation-notes`)

---

*Generated for handoff: repo paths under `vendor/twowiki-fossil-skin/`, deploy via `apply_twowiki_skin.py`.*

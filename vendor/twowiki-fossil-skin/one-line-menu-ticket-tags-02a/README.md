# one-line-menu-ticket-tags-02a

**Vendor drop** (Lovable / export) for **v8** look-and-feel: wider content measure, report “card” layout, tag-style variants, admin action bars — see `css.txt` header comment.

When `css.txt` is present, **`../apply_twowiki_skin.py`** uses it as **layer 3** (after `lovable_01a/css.txt` + `twowiki-fossil-th1-append.css`). The apply script also appends the **sortable markdown thead** tail from `01a/twowiki-fossil-skin-v6.css` until that block is merged into this drop. See **`../MERGE-DROPS.md`** for drop workflow.

## What is in the drop

| File | Role | Safe to paste into Fossil `config` as-is? |
|------|------|-------------------------------------------|
| `css.txt` | Full **v8** stylesheet (same role as `01a/…/twowiki-fossil-skin-v6.css`) | **Use as last CSS layer** in the merged stack. Apply merges v6 sortable thead rules after this file. |
| `header.txt` | **Full HTML document** (DOCTYPE … `<div class="wiki">` …) | **No.** Fossil `header` is a **fragment**; our canonical pattern is `../lovable_01a/header.txt` (TH1 + `$mainmenu` + `hbmenu.js`). Steal **selectors / structure ideas** only; merge by hand. |
| `footer.txt` | Closes `.content`, `.wiki`, `</body></html>` | **No** for `footer` **config** — twoWiki uses **`../footer.th1`** (Mermaid, `/ticket/HASH` → `/tktview`, `rpage-rptview` JS). Never replace `footer.th1` with this file. |
| `details.txt` | Skin details | Compare to `../lovable_01a/details.txt` (pikchr keys); merge if the drop changed toggles. |

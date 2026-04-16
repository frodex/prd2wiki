# one-line-menu-ticket-tags-02a

**Vendor drop** (Lovable / export) for **v7** look-and-feel: wider content measure, report “card” layout, tag-style variants, admin action bars — see `css.txt` header comment.

This folder is **not** wired into `apply_twowiki_skin.py` until someone **merges** it with the existing stack (see **`../MERGE-DROPS.md`**).

## What is in the drop

| File | Role | Safe to paste into Fossil `config` as-is? |
|------|------|-------------------------------------------|
| `css.txt` | Full **v7** stylesheet (same role as `01a/…/twowiki-fossil-skin-v6.css`) | **Use as last CSS layer only** after diff vs v6 + `twowiki-fossil-th1-append.css`. Do not replace merged `config.css` without carrying forward repo-only tails (e.g. `.twowiki-sortable` thead rules). |
| `header.txt` | **Full HTML document** (DOCTYPE … `<div class="wiki">` …) | **No.** Fossil `header` is a **fragment**; our canonical pattern is `../lovable_01a/header.txt` (TH1 + `$mainmenu` + `hbmenu.js`). Steal **selectors / structure ideas** only; merge by hand. |
| `footer.txt` | Closes `.content`, `.wiki`, `</body></html>` | **No** for `footer` **config** — twoWiki uses **`../footer.th1`** (Mermaid, `/ticket/HASH` → `/tktview`, `rpage-rptview` JS). Never replace `footer.th1` with this file. |
| `details.txt` | Skin details | Compare to `../lovable_01a/details.txt` (pikchr keys); merge if v7 changed toggles. |

## Naming

Rename or copy `css.txt` to `twowiki-fossil-skin-v7.css` when promoting into the merge pipeline so apply order stays obvious (layer 3 = design).

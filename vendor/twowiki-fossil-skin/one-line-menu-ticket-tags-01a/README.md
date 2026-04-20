# one-line-menu-ticket-tags-01a

**Style layer** (palette, single-line header chrome, ticket/doc tables) — merged **last** into `config.css` by `../apply_twowiki_skin.py`.

- `twowiki-fossil-skin-v6.css` — canonical v6 stylesheet; wins over `lovable_01a/css.txt` and `twowiki-fossil-th1-append.css`.
- `twowiki-fossil-skin-v6.zip` — optional Lovable export snapshot (`css.txt`, `header.txt`, `footer.txt`, `details.txt`). **Do not** replace `twowiki-fossil-skin-v6.css` with the zip’s `css.txt` blindly: the checked-in `v6.css` adds repo-only rules (e.g. `.twowiki-sortable` thead hover) that tie to `ticket-viewpage.th1`. If you refresh from the zip, merge those trailing rules back in.

See **`../SKIN-LAYERING.md`** for full merge rules and what MUST stay in TH1/footer for behavior.

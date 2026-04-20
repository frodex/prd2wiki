# Fossil markdown — first `#` line and ticket bodies

## What Fossil does (documented)

From [Fossil: Markdown Overview](https://fossil-scm.org/home/doc/trunk/src/markdown.md) (section *Special Features For Fossil*):

> For documents that begin with a top-level heading (ex: `# heading #`), **the heading is omitted from the body** of the document and **becomes the document title** displayed at the top of the Fossil page.

The TH1 `markdown` command returns a **list**: element **0** is that extracted title (plain text), element **1** is the **remaining HTML** for the body.

Wiki skins wire element 0 into page chrome. **Ticket view** (`ticket-viewpage.th1`) historically rendered **only** `[lindex $mdout 1]`, so the first-line heading **vanished** from the ticket while the ticket’s own `<h1>` still showed the **ticket title field** — often not the same string as the in-body `# …` line.

## What else can look “stripped”

- **Raw HTML** in markdown may be filtered by Fossil’s [safe-html](https://fossil-scm.org/home/help/safe-html) rules in some contexts (tags/attributes not on the allowlist).
- **Tables**: Fossil is not GFM; wrong delimiter rows break tables (see `FOSSIL-TICKET-MARKDOWN-TABLES.md`).
- **`untaint`** on rendered HTML is required for Fossil to emit user HTML from TH1; behavior is Fossil-version-specific.

## Mitigation in this skin

`ticket-viewpage.th1` defines `twowiki_emit_markdown_fragment`: it emits **`[lindex $mdout 0]`** as **`<h2 class="twowiki-md-from-atx-h1">…</h2>`** (not a second page `<h1>`, for a sane heading outline), then **`[untaint [lindex $mdout 1]]`**. Same for section fields and comment bodies.

Authors can still use **`##` as the first heading** if they want no extra line at the top of the prose block.

## Apply

Re-run `apply_twowiki_skin.py` after editing `ticket-viewpage.th1` or this note.

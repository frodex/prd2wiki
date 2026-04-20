# Fossil ticket markdown — tables (not GitHub GFM)

Fossil’s built-in markdown is **not** GitHub-flavored pipe tables. The delimiter row is **not** `|-----|-------|`.

From [Fossil: Markdown Overview — Tables](https://fossil-scm.org/home/doc/trunk/src/markdown.md):

```text
| Header 1     | Header 2    | Header 3      |
----------------------------------------------
| Row 1 Col 1  | Row 1 Col 2 | Row 1 Col 3   |
```

**Rule:** the line after the header must be a **horizontal rule** — a run of **`-` characters** (optionally with `:` alignment markers on later rows). It is **not** a pipe-separated row of dashes.

If you use a GFM-style delimiter (`|-----|-------|`), Fossil treats it as **normal table data**. You then get one “table” whose first body row is all dashes, and the remaining pipe rows can be parsed as **separate tiny tables** (the broken matrix you see on `/tktview`).

## Working “Sortable table smoke” matrix (paste into ticket `section_overview` / body)

Use a **dash-only** separator line (length is not magic; match roughly the table width, at least a few dozen `-`):

```markdown
# Sortable table smoke

| # | Label | Qty | Owner |
------------------------------------------
| 1 | Item-01 | 97 | Bob2 |
| 2 | Item-02 | 94 | Carol3 |
| 3 | Item-03 | 91 | Alice4 |
| 4 | Item-04 | 88 | Bob3 |
| 5 | Item-05 | 85 | Carol4 |
| 6 | Item-06 | 82 | Alice5 |
| 7 | Item-07 | 79 | Bob4 |
| 8 | Item-08 | 76 | Carol5 |
| 9 | Item-09 | 73 | Alice6 |
| 10 | Item-10 | 70 | Bob1 |
| 11 | Item-11 | 67 | Carol2 |
| 12 | Item-12 | 64 | Alice3 |
| 13 | Item-13 | 61 | Bob5 |
| 14 | Item-14 | 58 | Carol6 |
| 15 | Item-15 | 55 | Alice7 |
| 16 | Item-16 | 52 | Bob3 |
| 17 | Item-17 | 49 | Carol4 |
| 18 | Item-18 | 46 | Alice5 |
```

Notes:

- Renamed first column from `row` to `#` to avoid confusion with prose; optional.
- **No blank lines** between header, separator, and body rows.
- Do **not** indent the table with four spaces (that becomes a code block).

## Qualifying / sort tests

After the table is **one** `md-table`, the existing ticket-view `enableSortableTables()` logic applies: one matrix, one sortable table (with multiple body rows).

## Cross-check

[Pandoc](https://pandoc.org) and GitHub may accept GFM `| --- |` delimiters; **Fossil will not** treat them as delimiters. Always test in `/tktview` or `fossil test-markdown` on your Fossil version.

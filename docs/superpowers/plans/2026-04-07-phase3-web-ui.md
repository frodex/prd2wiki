# Phase 3: Web UI — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a browser-based WYSIWYG wiki interface using Milkdown (markdown-native editor), with submission mode buttons (verbatim/conform/integrate), diff preview, page navigation, and reference trees.

**Architecture:** Server-rendered HTML from the Go binary. Milkdown loaded from CDN (no frontend build system). The Go server serves static assets and HTML templates, proxying to the existing REST API. Minimal JS — just Milkdown + a thin glue layer for submission modes and diff preview.

**Tech Stack:** Go `html/template`, Milkdown (CDN), vanilla JS, CSS

---

## File Structure

```
internal/
├── web/
│   ├── web.go                 # Web handler — serves pages, editor, templates
│   ├── web_test.go
│   ├── templates/
│   │   ├── layout.html        # Base layout (nav, head, footer)
│   │   ├── page_view.html     # Page viewer (rendered markdown + metadata + refs)
│   │   ├── page_edit.html     # Page editor (Milkdown + submission buttons)
│   │   ├── page_list.html     # Page listing / project index
│   │   ├── diff_preview.html  # Diff preview modal
│   │   └── search.html        # Search results
│   └── static/
│       ├── style.css          # Wiki styling
│       └── wiki.js            # Milkdown init, submission handlers, diff display
```

---

### Task 1: Web Handler + Base Templates

**Files:**
- Create: `internal/web/web.go`
- Create: `internal/web/templates/layout.html`
- Create: `internal/web/templates/page_list.html`
- Create: `internal/web/web_test.go`

The web handler registers routes under `/` (not `/api/`):
- `GET /` → redirect to `/projects/default/pages`
- `GET /projects/{project}/pages` → page listing
- `GET /projects/{project}/pages/{id}` → view page
- `GET /projects/{project}/pages/{id}/edit` → edit page
- `GET /projects/{project}/pages/new` → new page form
- `POST /projects/{project}/pages/{id}/submit` → submit edit (calls API internally)
- `GET /projects/{project}/search?q=...` → search

The web handler calls the REST API internally (same process, direct function call — not HTTP). It needs access to the Server's repos, searcher, and librarians.

**layout.html** — base template with:
- Navigation: project name, links to page list, search, new page
- Main content block
- CSS link + Milkdown CDN scripts in head

**page_list.html** — table of all pages with id, title, type, status, trust level. Links to view each page.

Test: verify page list handler returns 200 with HTML.

- [ ] Implement and commit

### Task 2: Page Viewer

**Files:**
- Create: `internal/web/templates/page_view.html`
- Modify: `internal/web/web.go` — add view handler

Renders a wiki page:
- Title and metadata bar (type, status, trust level, creator, dates)
- Rendered markdown body (use goldmark to render to HTML)
- Tags displayed as badges
- Hard references section (collapsible tree — fetch from API)
- Soft references section (placeholder — requires vector DB with real embedder)
- Edit button linking to edit page
- Challenge/status indicators

- [ ] Implement and commit

### Task 3: Page Editor with Milkdown

**Files:**
- Create: `internal/web/templates/page_edit.html`
- Create: `internal/web/static/wiki.js`
- Create: `internal/web/static/style.css`
- Modify: `internal/web/web.go` — add edit/new/submit handlers

The editor page:
- Milkdown WYSIWYG editor initialized with page content (or empty for new)
- Frontmatter fields as form inputs above the editor (id, title, type, status, tags)
- Three submission buttons at the bottom:
  - `[Do Not Mutate]` → intent=verbatim
  - `[Correct & Format]` → intent=conform
  - `[Reason & Merge]` → intent=integrate
- Submit sends JSON to the API, gets back result with issues/warnings
- If conform/integrate: show diff preview before final save

**wiki.js:**
- Initialize Milkdown editor on the page
- Handle submission button clicks
- For conform/integrate: POST to API, show diff preview, wait for confirm
- Display validation issues/warnings

- [ ] Implement and commit

### Task 4: Diff Preview

**Files:**
- Create: `internal/web/templates/diff_preview.html` (or inline in page_edit.html)
- Modify: `internal/web/static/wiki.js` — add diff display

When conform or integrate returns changes:
- Show a modal/panel with the diff (what was changed, field by field)
- Accept / Reject / Save As-Is buttons
- Accept → confirm the submission
- Reject → go back to editing
- Save As-Is → resubmit as verbatim

- [ ] Implement and commit

### Task 5: Reference Trees in Page View

**Files:**
- Modify: `internal/web/static/wiki.js` — add tree expand/collapse
- Modify: `internal/web/templates/page_view.html` — reference tree section

Fetch references from `/api/projects/{project}/pages/{id}/references?depth=1`. Display as expandable tree. Click ▶ to expand (fetches deeper levels via API). Show status indicators (✓ valid, ⚠ stale).

- [ ] Implement and commit

### Task 6: Search Page

**Files:**
- Create: `internal/web/templates/search.html`
- Modify: `internal/web/web.go` — add search handler

Search form that queries the API. Shows results with page titles, types, relevance. Links to view each result.

- [ ] Implement and commit

### Task 7: Wire Web Handler into main.go

**Files:**
- Modify: `cmd/prd2wiki/main.go` — register web routes alongside API routes
- Modify: `internal/api/server.go` — expose Handler() for composition

The main mux serves both:
- `/api/...` → API handlers (JSON)
- `/...` → Web handlers (HTML)
- `/static/...` → Static file serving

- [ ] Implement and commit

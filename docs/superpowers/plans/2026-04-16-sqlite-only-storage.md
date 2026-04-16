# Phase 3: SQLite-Only Storage — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace git bare repos + `.link` files + tree scanner with SQLite as the sole storage engine for wiki pages. Two tables: `pages` (current state) and `page_bodies` (append-only history).

**Architecture:** New `internal/store` package owns all page CRUD. Librarian calls `store` instead of git. API handlers and web handlers call `store` for reads. History/diff reads from `page_bodies`. Git, `.link` files, tree scanner, and IndexHolder are removed entirely.

**Tech Stack:** Go, SQLite (modernc.org/sqlite — already a dependency), `database/sql`

**Design brief:** [DESIGN BRIEF: Phase 3 — SQLite-Only Storage](/prd2wiki/design-brief-phase-3-write-core-unification)

**Pre-requisite:** Branch from `main` at or after `c5c4b65`. New worktree per plan §0.2.

---

## File Structure

### New files

| File | Responsibility |
|------|---------------|
| `internal/store/store.go` | `PageStore` struct + constructor. Opens DB, runs migrations, sets WAL mode. |
| `internal/store/schema.go` | `CREATE TABLE` statements for `pages` + `page_bodies`. Migration logic. |
| `internal/store/create.go` | `Create(ctx, CreateCommand) → (*Page, error)` |
| `internal/store/read.go` | `GetByTreePath`, `GetByUUID`, `List`, `ListByProject` |
| `internal/store/update.go` | `Update(ctx, UpdateCommand) → (*Page, error)` — merge semantics |
| `internal/store/delete.go` | `Delete(ctx, uuid)` |
| `internal/store/history.go` | `History(uuid, limit)`, `BodyAtVersion(bodyID)`, `Diff(bodyID1, bodyID2)` |
| `internal/store/search.go` | FTS wrapper — `Search(project, query, filters)` |
| `internal/store/types.go` | `Page`, `PageBody`, `CreateCommand`, `UpdateCommand` structs |
| `internal/store/store_test.go` | Tests for all CRUD + history + search |
| `cmd/prd2wiki-migrate-sqlite/main.go` | One-shot migration: reads git repos + `.link` files → inserts into `pages` + `page_bodies` |

### Files to modify

| File | Change |
|------|--------|
| `internal/api/server.go` | Replace `repos`, `treeHolder`, `indexer`, `search` with `*store.PageStore` |
| `internal/api/pages.go` | Handlers call `store.Create`/`store.GetByUUID`/`store.Update`/`store.Delete` |
| `internal/api/tree_api.go` | Handlers call `store.GetByTreePath`/`store.Create`/`store.Update`/`store.Delete` |
| `internal/api/lifecycle.go` | `deprecate`/`approve`/`restore` call `store.Update` with status change |
| `internal/api/history.go` | Calls `store.History`/`store.BodyAtVersion` instead of git |
| `internal/api/attachments.go` | Attachments need a new storage strategy (see Task 10) |
| `internal/api/search.go` | Calls `store.Search` |
| `internal/api/auth_test.go` | Update test setup to use `store.PageStore` instead of git repos |
| `internal/api/pages_test.go` | Same |
| `internal/librarian/librarian.go` | `Submit` writes to `store` instead of git. Remove `repo` field. |
| `internal/web/handler.go` | Replace `repos`, `treeHolder` with `*store.PageStore` |
| `internal/web/pages.go` | Read from `store` |
| `internal/web/list.go` | Read from `store` |
| `internal/web/home.go` | Read from `store` |
| `internal/web/history.go` | Read from `store.History` |
| `internal/web/nav.go` | Sidebar built from `store.ListByProject` instead of tree scanner |
| `internal/mcp/tools.go` | `toolPropose` drops `.link` management, calls tree API or `store` |
| `internal/mcp/client.go` | May switch to tree API URLs |
| `internal/app/app.go` | Remove git repo init, tree scan, index rebuild. Create `store.PageStore`. |
| `cmd/prd2wiki/main.go` | Wiring changes if needed |

### Files to remove (after migration verified)

| File | Why |
|------|-----|
| `internal/git/*.go` | Bare repo operations — replaced by SQL |
| `internal/tree/*.go` | Scanner, IndexHolder, `.link` files — replaced by SQL columns |
| `internal/pagepath/*.go` | Path resolution (hash-prefix fallback) — replaced by SQL lookup |
| `internal/index/indexer.go` | `RebuildFromRepo` + `IndexPage` — FTS now updated inline by `store` |
| `internal/api/projects_redirect.go` | Legacy redirect — project API going away |
| `internal/api/tree_multipart.go` | Multipart handler for tree API — simplify to one JSON path |

---

## Task 1: Create `internal/store` package — schema + constructor

**Files:**
- Create: `internal/store/schema.go`
- Create: `internal/store/store.go`
- Create: `internal/store/types.go`
- Test: `internal/store/store_test.go`

- [ ] **Step 1: Write types**

Create `internal/store/types.go`:

```go
package store

import "time"

// Page is a live wiki page — one row in the pages table.
type Page struct {
	UUID       string    `json:"uuid"`
	Slug       string    `json:"slug"`
	TreePath   string    `json:"tree_path"`
	Project    string    `json:"project"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	BodyID     int64     `json:"body_id"`
	Type       string    `json:"type"`
	Status     string    `json:"status"`
	Tags       []string  `json:"tags"`
	Author     string    `json:"author"`
	CreatedAt  time.Time `json:"created_at"`
	ModifiedAt time.Time `json:"modified_at"`
}

// PageBody is one version of a page — append-only history.
type PageBody struct {
	ID        int64     `json:"id"`
	PageUUID  string    `json:"page_uuid"`
	Body      string    `json:"body"`
	Title     string    `json:"title"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Tags      []string  `json:"tags"`
	Author    string    `json:"author"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateCommand is the input for creating a page.
type CreateCommand struct {
	Title   string
	Slug    string   // optional — derived from title if empty
	Project string   // tree project name, e.g. "prd2wiki"
	Type    string   // default "concept"
	Status  string   // default "draft"
	Tags    []string
	Body    string
	Author  string
	Intent  string   // verbatim, conform, integrate
	Message string   // commit message equivalent
}

// UpdateCommand is the input for updating a page.
// Empty fields mean "keep current value" (merge semantics).
type UpdateCommand struct {
	Title   string
	Type    string
	Status  string
	Tags    []string // nil = keep current; empty slice = clear tags
	Body    string
	Author  string
	Intent  string
	Message string
}
```

- [ ] **Step 2: Write schema**

Create `internal/store/schema.go`:

```go
package store

import "database/sql"

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS pages (
			uuid        TEXT PRIMARY KEY,
			slug        TEXT NOT NULL,
			tree_path   TEXT NOT NULL UNIQUE,
			project     TEXT NOT NULL,
			title       TEXT NOT NULL,
			body        TEXT NOT NULL DEFAULT '',
			body_id     INTEGER,
			type        TEXT NOT NULL DEFAULT 'concept',
			status      TEXT NOT NULL DEFAULT 'draft',
			tags        TEXT NOT NULL DEFAULT '[]',
			author      TEXT NOT NULL DEFAULT 'anonymous',
			created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			modified_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE TABLE IF NOT EXISTS page_bodies (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			page_uuid   TEXT NOT NULL,
			body        TEXT NOT NULL,
			title       TEXT NOT NULL DEFAULT '',
			type        TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT '',
			tags        TEXT NOT NULL DEFAULT '[]',
			author      TEXT NOT NULL,
			message     TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_pages_tree_path ON pages(tree_path)`,
		`CREATE INDEX IF NOT EXISTS idx_pages_project ON pages(project)`,
		`CREATE INDEX IF NOT EXISTS idx_pages_slug ON pages(slug, project)`,
		`CREATE INDEX IF NOT EXISTS idx_pages_type ON pages(type)`,
		`CREATE INDEX IF NOT EXISTS idx_pages_status ON pages(status)`,
		`CREATE INDEX IF NOT EXISTS idx_page_bodies_uuid ON page_bodies(page_uuid, created_at DESC)`,
		// FTS table — same structure as existing, reused for search.
		`CREATE VIRTUAL TABLE IF NOT EXISTS pages_fts USING fts5(
			uuid UNINDEXED, title, body, tags
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 3: Write store constructor**

Create `internal/store/store.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// PageStore is the single source of truth for wiki pages.
type PageStore struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database and runs migrations.
func New(dbPath string) (*PageStore, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &PageStore{db: db}, nil
}

// NewFromDB wraps an existing *sql.DB (for sharing with auth/FTS).
func NewFromDB(db *sql.DB) (*PageStore, error) {
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &PageStore{db: db}, nil
}

// Close closes the database connection.
func (s *PageStore) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection for shared use (auth, legacy FTS).
func (s *PageStore) DB() *sql.DB {
	return s.db
}

// slugFromTitle converts a title to a URL-friendly slug.
func slugFromTitle(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	// Collapse multiple dashes.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
```

- [ ] **Step 4: Write test — store opens and migrates**

Add to `internal/store/store_test.go`:

```go
package store

import (
	"os"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *PageStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewStore(t *testing.T) {
	s := testStore(t)
	// Verify tables exist by querying them.
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM pages").Scan(&count); err != nil {
		t.Fatalf("query pages: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 pages, got %d", count)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM page_bodies").Scan(&count); err != nil {
		t.Fatalf("query page_bodies: %v", err)
	}
}

func TestSlugFromTitle(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Hello World", "hello-world"},
		{"API Unification — Phase 3", "api-unification--phase-3"},
		{"  spaces  ", "spaces"},
		{"UPPER CASE", "upper-case"},
	}
	for _, tc := range tests {
		got := slugFromTitle(tc.in)
		// Allow collapsed dashes
		if got != tc.want && got != strings.ReplaceAll(tc.want, "--", "-") {
			t.Errorf("slugFromTitle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/store/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/store/
git commit -m "feat(store): page store schema, types, and constructor

New internal/store package — SQLite-only page storage.
Two tables: pages (current state) + page_bodies (append-only history).
WAL mode for concurrent read/write.

Phase 3 of API unification — SQLite-only storage."
```

---

## Task 2: Implement Create

**Files:**
- Create: `internal/store/create.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: Write failing test**

Add to `internal/store/store_test.go`:

```go
func TestCreate(t *testing.T) {
	s := testStore(t)

	page, err := s.Create(context.Background(), CreateCommand{
		Title:   "Test Page",
		Project: "prd2wiki",
		Type:    "reference",
		Status:  "draft",
		Tags:    []string{"test", "demo"},
		Body:    "# Hello\n\nThis is a test.",
		Author:  "test@prd2wiki",
		Message: "initial create",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if page.UUID == "" {
		t.Error("expected non-empty UUID")
	}
	if page.Slug != "test-page" {
		t.Errorf("slug: got %q, want %q", page.Slug, "test-page")
	}
	if page.TreePath != "prd2wiki/test-page" {
		t.Errorf("tree_path: got %q, want %q", page.TreePath, "prd2wiki/test-page")
	}
	if page.Title != "Test Page" {
		t.Errorf("title: got %q", page.Title)
	}
	if page.Body != "# Hello\n\nThis is a test." {
		t.Errorf("body mismatch")
	}
	if page.BodyID == 0 {
		t.Error("expected non-zero body_id")
	}

	// Verify page_bodies has one row.
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM page_bodies WHERE page_uuid = ?", page.UUID).Scan(&count)
	if count != 1 {
		t.Errorf("page_bodies count: got %d, want 1", count)
	}

	// Verify FTS indexed.
	var ftsCount int
	s.db.QueryRow("SELECT COUNT(*) FROM pages_fts WHERE title MATCH 'Test'").Scan(&ftsCount)
	if ftsCount != 1 {
		t.Errorf("FTS count: got %d, want 1", ftsCount)
	}
}

func TestCreateDuplicateSlug(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_, err := s.Create(ctx, CreateCommand{Title: "My Page", Project: "proj", Body: "a", Author: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// Same title → slug collision → should get uniquified slug.
	p2, err := s.Create(ctx, CreateCommand{Title: "My Page", Project: "proj", Body: "b", Author: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if p2.Slug == "my-page" {
		t.Error("expected uniquified slug, got same as first")
	}
	if p2.TreePath == "proj/my-page" {
		t.Error("expected uniquified tree_path")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/... -run TestCreate -v`
Expected: FAIL — `Create` not defined

- [ ] **Step 3: Implement Create**

Create `internal/store/create.go`:

```go
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Create inserts a new page and its initial body version. Returns the created page.
func (s *PageStore) Create(ctx context.Context, cmd CreateCommand) (*Page, error) {
	pageUUID := uuid.New().String()

	slug := strings.TrimSpace(cmd.Slug)
	if slug == "" {
		slug = slugFromTitle(cmd.Title)
	}
	if slug == "" {
		slug = pageUUID[:8]
	}

	project := strings.TrimSpace(cmd.Project)
	if project == "" {
		return nil, fmt.Errorf("project is required")
	}
	if strings.TrimSpace(cmd.Title) == "" {
		return nil, fmt.Errorf("title is required")
	}

	typ := cmd.Type
	if typ == "" {
		typ = "concept"
	}
	status := cmd.Status
	if status == "" {
		status = "draft"
	}
	author := cmd.Author
	if author == "" {
		author = "anonymous"
	}

	tagsJSON, _ := json.Marshal(cmd.Tags)
	if cmd.Tags == nil {
		tagsJSON = []byte("[]")
	}

	// Uniquify slug within project.
	slug, err := s.uniqueSlug(ctx, project, slug)
	if err != nil {
		return nil, err
	}

	treePath := project + "/" + slug

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Insert body version.
	res, err := tx.ExecContext(ctx,
		`INSERT INTO page_bodies (page_uuid, body, title, type, status, tags, author, message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		pageUUID, cmd.Body, cmd.Title, typ, status, string(tagsJSON), author, cmd.Message)
	if err != nil {
		return nil, fmt.Errorf("insert page_bodies: %w", err)
	}
	bodyID, _ := res.LastInsertId()

	// Insert page.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO pages (uuid, slug, tree_path, project, title, body, body_id, type, status, tags, author)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pageUUID, slug, treePath, project, cmd.Title, cmd.Body, bodyID, typ, status, string(tagsJSON), author)
	if err != nil {
		return nil, fmt.Errorf("insert pages: %w", err)
	}

	// Update FTS.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO pages_fts (uuid, title, body, tags) VALUES (?, ?, ?, ?)`,
		pageUUID, cmd.Title, cmd.Body, strings.Join(cmd.Tags, " "))
	if err != nil {
		return nil, fmt.Errorf("insert FTS: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetByUUID(ctx, pageUUID)
}

// uniqueSlug returns a slug that doesn't collide within the project.
func (s *PageStore) uniqueSlug(ctx context.Context, project, slug string) (string, error) {
	base := slug
	for i := 0; i < 100; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i)
		}
		treePath := project + "/" + candidate
		var exists int
		err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pages WHERE tree_path = ?", treePath).Scan(&exists)
		if err != nil {
			return "", err
		}
		if exists == 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not uniquify slug %q after 100 attempts", slug)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/... -run TestCreate -v`
Expected: PASS (both TestCreate and TestCreateDuplicateSlug)

- [ ] **Step 5: Commit**

```bash
git add internal/store/create.go internal/store/store_test.go
git commit -m "feat(store): Create — insert page + body + FTS in one transaction"
```

---

## Task 3: Implement Read operations

**Files:**
- Create: `internal/store/read.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: Write failing tests**

Add to `store_test.go`:

```go
func TestGetByTreePath(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	created, _ := s.Create(ctx, CreateCommand{Title: "Read Test", Project: "proj", Body: "body", Author: "x"})

	page, err := s.GetByTreePath(ctx, "proj/read-test")
	if err != nil {
		t.Fatalf("GetByTreePath: %v", err)
	}
	if page.UUID != created.UUID {
		t.Errorf("UUID mismatch: got %q, want %q", page.UUID, created.UUID)
	}
	if page.Body != "body" {
		t.Errorf("body: got %q", page.Body)
	}

	// Not found.
	_, err = s.GetByTreePath(ctx, "proj/nonexistent")
	if err == nil {
		t.Error("expected error for missing page")
	}
}

func TestGetByUUID(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	created, _ := s.Create(ctx, CreateCommand{Title: "UUID Test", Project: "proj", Body: "b", Author: "x"})

	page, err := s.GetByUUID(ctx, created.UUID)
	if err != nil {
		t.Fatalf("GetByUUID: %v", err)
	}
	if page.TreePath != "proj/uuid-test" {
		t.Errorf("tree_path: got %q", page.TreePath)
	}
}

func TestListByProject(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.Create(ctx, CreateCommand{Title: "A", Project: "p1", Body: "a", Author: "x"})
	s.Create(ctx, CreateCommand{Title: "B", Project: "p1", Body: "b", Author: "x"})
	s.Create(ctx, CreateCommand{Title: "C", Project: "p2", Body: "c", Author: "x"})

	pages, err := s.ListByProject(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Errorf("expected 2 pages in p1, got %d", len(pages))
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

Run: `go test ./internal/store/... -run "GetBy|List" -v`

- [ ] **Step 3: Implement reads**

Create `internal/store/read.go`:

```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// GetByTreePath returns a page by its full tree URL path.
func (s *PageStore) GetByTreePath(ctx context.Context, treePath string) (*Page, error) {
	return s.scanPage(s.db.QueryRowContext(ctx,
		`SELECT uuid, slug, tree_path, project, title, body, body_id,
		        type, status, tags, author, created_at, modified_at
		 FROM pages WHERE tree_path = ?`, treePath))
}

// GetByUUID returns a page by its UUID.
func (s *PageStore) GetByUUID(ctx context.Context, uuid string) (*Page, error) {
	return s.scanPage(s.db.QueryRowContext(ctx,
		`SELECT uuid, slug, tree_path, project, title, body, body_id,
		        type, status, tags, author, created_at, modified_at
		 FROM pages WHERE uuid = ?`, uuid))
}

// ListByProject returns all pages in a project, ordered by title.
func (s *PageStore) ListByProject(ctx context.Context, project string) ([]Page, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT uuid, slug, tree_path, project, title, body, body_id,
		        type, status, tags, author, created_at, modified_at
		 FROM pages WHERE project = ? ORDER BY title`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanPages(rows)
}

// ListAll returns all pages ordered by project then title.
func (s *PageStore) ListAll(ctx context.Context) ([]Page, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT uuid, slug, tree_path, project, title, body, body_id,
		        type, status, tags, author, created_at, modified_at
		 FROM pages ORDER BY project, title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanPages(rows)
}

func (s *PageStore) scanPage(row *sql.Row) (*Page, error) {
	var p Page
	var tagsJSON string
	var created, modified string
	err := row.Scan(&p.UUID, &p.Slug, &p.TreePath, &p.Project, &p.Title, &p.Body, &p.BodyID,
		&p.Type, &p.Status, &tagsJSON, &p.Author, &created, &modified)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("page not found")
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(tagsJSON), &p.Tags)
	if p.Tags == nil {
		p.Tags = []string{}
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	p.ModifiedAt, _ = time.Parse(time.RFC3339Nano, modified)
	return &p, nil
}

func (s *PageStore) scanPages(rows *sql.Rows) ([]Page, error) {
	var pages []Page
	for rows.Next() {
		var p Page
		var tagsJSON string
		var created, modified string
		if err := rows.Scan(&p.UUID, &p.Slug, &p.TreePath, &p.Project, &p.Title, &p.Body, &p.BodyID,
			&p.Type, &p.Status, &tagsJSON, &p.Author, &created, &modified); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(tagsJSON), &p.Tags)
		if p.Tags == nil {
			p.Tags = []string{}
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		p.ModifiedAt, _ = time.Parse(time.RFC3339Nano, modified)
		pages = append(pages, p)
	}
	return pages, rows.Err()
}
```

- [ ] **Step 4: Run tests — expect PASS**

Run: `go test ./internal/store/... -v`

- [ ] **Step 5: Commit**

```bash
git add internal/store/read.go internal/store/store_test.go
git commit -m "feat(store): Read — GetByTreePath, GetByUUID, ListByProject, ListAll"
```

---

## Task 4: Implement Update with merge semantics

**Files:**
- Create: `internal/store/update.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestUpdate(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	page, _ := s.Create(ctx, CreateCommand{
		Title: "Original", Project: "proj", Type: "reference", Status: "approved",
		Tags: []string{"a"}, Body: "original body", Author: "alice",
	})

	// Update body only — other fields should be preserved (merge semantics).
	updated, err := s.Update(ctx, page.UUID, UpdateCommand{
		Body: "new body", Author: "bob", Message: "fix typo",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Body != "new body" {
		t.Errorf("body: got %q", updated.Body)
	}
	if updated.Title != "Original" {
		t.Errorf("title clobbered: got %q", updated.Title)
	}
	if updated.Type != "reference" {
		t.Errorf("type clobbered: got %q", updated.Type)
	}
	if updated.Status != "approved" {
		t.Errorf("status clobbered: got %q", updated.Status)
	}

	// Verify history has 2 versions (initial + update).
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM page_bodies WHERE page_uuid = ?", page.UUID).Scan(&count)
	if count != 2 {
		t.Errorf("page_bodies count: got %d, want 2", count)
	}
}

func TestUpdateExplicitFields(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	page, _ := s.Create(ctx, CreateCommand{
		Title: "T", Project: "proj", Type: "concept", Status: "draft",
		Body: "b", Author: "x",
	})

	// Explicitly change status.
	updated, err := s.Update(ctx, page.UUID, UpdateCommand{
		Status: "approved", Author: "reviewer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "approved" {
		t.Errorf("status: got %q, want approved", updated.Status)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement Update**

Create `internal/store/update.go`:

```go
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Update modifies a page using merge semantics: empty fields keep current values.
// Appends the previous state to page_bodies before overwriting.
func (s *PageStore) Update(ctx context.Context, uuid string, cmd UpdateCommand) (*Page, error) {
	current, err := s.GetByUUID(ctx, uuid)
	if err != nil {
		return nil, fmt.Errorf("page not found: %w", err)
	}

	// Merge: non-empty command fields override current.
	title := current.Title
	if strings.TrimSpace(cmd.Title) != "" {
		title = cmd.Title
	}
	typ := current.Type
	if cmd.Type != "" {
		typ = cmd.Type
	}
	status := current.Status
	if cmd.Status != "" {
		status = cmd.Status
	}
	body := current.Body
	if cmd.Body != "" {
		body = cmd.Body
	}
	tags := current.Tags
	if cmd.Tags != nil {
		tags = cmd.Tags
	}
	author := cmd.Author
	if author == "" {
		author = current.Author
	}

	tagsJSON, _ := json.Marshal(tags)
	currentTagsJSON, _ := json.Marshal(current.Tags)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Snapshot current state into history.
	res, err := tx.ExecContext(ctx,
		`INSERT INTO page_bodies (page_uuid, body, title, type, status, tags, author, message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid, current.Body, current.Title, current.Type, current.Status,
		string(currentTagsJSON), author, cmd.Message)
	if err != nil {
		return nil, fmt.Errorf("insert history: %w", err)
	}
	bodyID, _ := res.LastInsertId()

	// Update live page.
	_, err = tx.ExecContext(ctx,
		`UPDATE pages SET title=?, body=?, body_id=?, type=?, status=?, tags=?, author=?,
		        modified_at=strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		 WHERE uuid = ?`,
		title, body, bodyID, typ, status, string(tagsJSON), author, uuid)
	if err != nil {
		return nil, fmt.Errorf("update page: %w", err)
	}

	// Update FTS.
	tx.ExecContext(ctx, `DELETE FROM pages_fts WHERE uuid = ?`, uuid)
	tx.ExecContext(ctx,
		`INSERT INTO pages_fts (uuid, title, body, tags) VALUES (?, ?, ?, ?)`,
		uuid, title, body, strings.Join(tags, " "))

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetByUUID(ctx, uuid)
}
```

- [ ] **Step 4: Run tests — expect PASS**

Run: `go test ./internal/store/... -v`

- [ ] **Step 5: Commit**

```bash
git add internal/store/update.go internal/store/store_test.go
git commit -m "feat(store): Update — merge semantics, append history before overwrite"
```

---

## Task 5: Implement Delete

**Files:**
- Create: `internal/store/delete.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestDelete(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	page, _ := s.Create(ctx, CreateCommand{Title: "Gone", Project: "proj", Body: "b", Author: "x"})

	err := s.Delete(ctx, page.UUID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Page gone from live table.
	_, err = s.GetByUUID(ctx, page.UUID)
	if err == nil {
		t.Error("expected page not found after delete")
	}

	// History preserved.
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM page_bodies WHERE page_uuid = ?", page.UUID).Scan(&count)
	if count == 0 {
		t.Error("expected history to be preserved after delete")
	}

	// FTS cleared.
	var fts int
	s.db.QueryRow("SELECT COUNT(*) FROM pages_fts WHERE uuid = ?", page.UUID).Scan(&fts)
	if fts != 0 {
		t.Error("expected FTS entry removed after delete")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement Delete**

Create `internal/store/delete.go`:

```go
package store

import (
	"context"
	"fmt"
)

// Delete removes a page from the live table. History rows in page_bodies are preserved.
func (s *PageStore) Delete(ctx context.Context, uuid string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `DELETE FROM pages WHERE uuid = ?`, uuid)
	if err != nil {
		return fmt.Errorf("delete page: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("page not found")
	}

	tx.ExecContext(ctx, `DELETE FROM pages_fts WHERE uuid = ?`, uuid)

	return tx.Commit()
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `go test ./internal/store/... -v`

- [ ] **Step 5: Commit**

```bash
git add internal/store/delete.go internal/store/store_test.go
git commit -m "feat(store): Delete — remove from pages + FTS, preserve history"
```

---

## Task 6: Implement History and Diff

**Files:**
- Create: `internal/store/history.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestHistory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	page, _ := s.Create(ctx, CreateCommand{Title: "V1", Project: "proj", Body: "v1", Author: "x"})
	s.Update(ctx, page.UUID, UpdateCommand{Body: "v2", Author: "x", Message: "update 1"})
	s.Update(ctx, page.UUID, UpdateCommand{Body: "v3", Author: "x", Message: "update 2"})

	history, err := s.History(ctx, page.UUID, 10)
	if err != nil {
		t.Fatal(err)
	}
	// 3 entries: initial create + 2 updates (each update snapshots previous state).
	if len(history) != 3 {
		t.Fatalf("history count: got %d, want 3", len(history))
	}
	// Most recent first.
	if history[0].Body != "v2" {
		t.Errorf("history[0].Body: got %q, want v2", history[0].Body)
	}
}

func TestBodyAtVersion(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	page, _ := s.Create(ctx, CreateCommand{Title: "V", Project: "proj", Body: "original", Author: "x"})

	body, err := s.BodyAtVersion(ctx, page.BodyID)
	if err != nil {
		t.Fatal(err)
	}
	if body.Body != "original" {
		t.Errorf("body: got %q", body.Body)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement History**

Create `internal/store/history.go`:

```go
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// History returns version history for a page, most recent first.
func (s *PageStore) History(ctx context.Context, pageUUID string, limit int) ([]PageBody, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, page_uuid, body, title, type, status, tags, author, message, created_at
		 FROM page_bodies WHERE page_uuid = ?
		 ORDER BY created_at DESC LIMIT ?`, pageUUID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PageBody
	for rows.Next() {
		var b PageBody
		var tagsJSON, created string
		if err := rows.Scan(&b.ID, &b.PageUUID, &b.Body, &b.Title, &b.Type, &b.Status,
			&tagsJSON, &b.Author, &b.Message, &created); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(tagsJSON), &b.Tags)
		if b.Tags == nil {
			b.Tags = []string{}
		}
		b.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		result = append(result, b)
	}
	return result, rows.Err()
}

// BodyAtVersion returns a single version by body ID.
func (s *PageStore) BodyAtVersion(ctx context.Context, bodyID int64) (*PageBody, error) {
	var b PageBody
	var tagsJSON, created string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, page_uuid, body, title, type, status, tags, author, message, created_at
		 FROM page_bodies WHERE id = ?`, bodyID).
		Scan(&b.ID, &b.PageUUID, &b.Body, &b.Title, &b.Type, &b.Status,
			&tagsJSON, &b.Author, &b.Message, &created)
	if err != nil {
		return nil, fmt.Errorf("version not found: %w", err)
	}
	json.Unmarshal([]byte(tagsJSON), &b.Tags)
	b.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return &b, nil
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add internal/store/history.go internal/store/store_test.go
git commit -m "feat(store): History — version list and body-at-version lookup"
```

---

## Task 7: Implement Search

**Files:**
- Create: `internal/store/search.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestSearch(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.Create(ctx, CreateCommand{Title: "Auth Guide", Project: "prd2wiki", Body: "how to authenticate", Author: "x", Tags: []string{"auth"}})
	s.Create(ctx, CreateCommand{Title: "Search Guide", Project: "prd2wiki", Body: "how to search", Author: "x"})

	results, err := s.Search(ctx, "prd2wiki", "authenticate", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "Auth Guide" {
		t.Errorf("title: got %q", results[0].Title)
	}
}

func TestSearchByType(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.Create(ctx, CreateCommand{Title: "A", Project: "p", Type: "reference", Body: "a", Author: "x"})
	s.Create(ctx, CreateCommand{Title: "B", Project: "p", Type: "plan", Body: "b", Author: "x"})

	results, err := s.ListByType(ctx, "p", "reference")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1, got %d", len(results))
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement Search**

Create `internal/store/search.go`:

```go
package store

import (
	"context"
	"encoding/json"
	"time"
)

// SearchResult is a page returned from a search query.
type SearchResult struct {
	UUID     string   `json:"uuid"`
	Title    string   `json:"title"`
	TreePath string   `json:"tree_path"`
	Type     string   `json:"type"`
	Status   string   `json:"status"`
	Tags     []string `json:"tags"`
	Project  string   `json:"project"`
	Snippet  string   `json:"snippet,omitempty"`
}

// Search performs FTS search within a project.
func (s *PageStore) Search(ctx context.Context, project, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.uuid, p.title, p.tree_path, p.type, p.status, p.tags, p.project,
		        snippet(pages_fts, 2, '<b>', '</b>', '...', 32) as snip
		 FROM pages_fts f
		 JOIN pages p ON p.uuid = f.uuid
		 WHERE pages_fts MATCH ? AND p.project = ?
		 ORDER BY rank
		 LIMIT ?`, query, project, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var tagsJSON string
		if err := rows.Scan(&r.UUID, &r.Title, &r.TreePath, &r.Type, &r.Status, &tagsJSON, &r.Project, &r.Snippet); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(tagsJSON), &r.Tags)
		results = append(results, r)
	}
	return results, rows.Err()
}

// ListByType returns pages in a project filtered by type.
func (s *PageStore) ListByType(ctx context.Context, project, typ string) ([]Page, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT uuid, slug, tree_path, project, title, body, body_id,
		        type, status, tags, author, created_at, modified_at
		 FROM pages WHERE project = ? AND type = ? ORDER BY title`, project, typ)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanPages(rows)
}

// ListByStatus returns pages in a project filtered by status.
func (s *PageStore) ListByStatus(ctx context.Context, project, status string) ([]Page, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT uuid, slug, tree_path, project, title, body, body_id,
		        type, status, tags, author, created_at, modified_at
		 FROM pages WHERE project = ? AND status = ? ORDER BY title`, project, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanPages(rows)
}

// ListByTag returns pages in a project that have a specific tag.
func (s *PageStore) ListByTag(ctx context.Context, project, tag string) ([]Page, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT uuid, slug, tree_path, project, title, body, body_id,
		        type, status, tags, author, created_at, modified_at
		 FROM pages WHERE project = ? AND EXISTS (
			SELECT 1 FROM json_each(tags) WHERE value = ?
		 ) ORDER BY title`, project, tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanPages(rows)
}

// Projects returns all distinct project names.
func (s *PageStore) Projects(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT project FROM pages ORDER BY project`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []string
	for rows.Next() {
		var p string
		rows.Scan(&p)
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// unused helper in this file but needed for type conversion
func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add internal/store/search.go internal/store/store_test.go
git commit -m "feat(store): Search — FTS, filter by type/status/tag, list projects"
```

---

## Task 8: Migration tool — git repos → SQLite

**Files:**
- Create: `cmd/prd2wiki-migrate-sqlite/main.go`

- [ ] **Step 1: Write the migration tool**

This is a one-shot script that reads every page from every git repo + every `.link` file and inserts into the new `pages` + `page_bodies` tables.

Create `cmd/prd2wiki-migrate-sqlite/main.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/store"
	"github.com/frodex/prd2wiki/internal/tree"
)

func main() {
	dataDir := envOrDefault("PRDWIKI_DATA_DIR", "./data")
	treeDir := envOrDefault("PRDWIKI_TREE_DIR", "./tree")
	dbPath := filepath.Join(dataDir, "index.db")

	log.Printf("migrating: data=%s tree=%s db=%s", dataDir, treeDir, dbPath)

	// Open the store (creates tables if needed).
	pageStore, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer pageStore.Close()

	// Scan tree to get .link → UUID → project mapping.
	treeIdx, err := tree.Scan(filepath.Clean(treeDir), filepath.Clean(dataDir))
	if err != nil {
		log.Fatalf("tree scan: %v", err)
	}

	// Build UUID → tree info map.
	type treeInfo struct {
		Slug     string
		TreePath string
		Project  string
	}
	treeMap := make(map[string]treeInfo)
	for _, ent := range treeIdx.AllPageEntries() {
		treeMap[ent.Page.UUID] = treeInfo{
			Slug:     ent.Page.Slug,
			TreePath: ent.Page.TreePath,
			Project:  ent.Project.Path,
		}
	}

	// Walk each git repo.
	var migrated, skipped, errors int
	ctx := context.Background()

	for _, proj := range treeIdx.Projects {
		repo, err := wgit.OpenRepo(dataDir, proj.RepoKey)
		if err != nil {
			log.Printf("WARN: cannot open repo %s: %v", proj.RepoKey, err)
			continue
		}

		branches, _ := repo.ListBranches()
		for _, branch := range branches {
			paths, err := repo.ListPages(branch)
			if err != nil {
				continue
			}

			for _, path := range paths {
				if !strings.HasSuffix(path, ".md") || strings.Contains(path, "_attachments") {
					continue
				}

				fm, body, err := repo.ReadPageWithMeta(branch, path)
				if err != nil || fm == nil {
					skipped++
					continue
				}

				// Check if already migrated.
				if existing, _ := pageStore.GetByUUID(ctx, fm.ID); existing != nil {
					skipped++
					continue
				}

				// Find tree info for this page.
				info, hasTree := treeMap[fm.ID]
				slug := info.Slug
				treePath := info.TreePath
				project := info.Project

				if !hasTree {
					// No .link file — derive from page ID.
					project = proj.Path
					slug = store.SlugFromTitle(fm.Title)
					if slug == "" {
						slug = fm.ID[:8]
					}
					treePath = project + "/" + slug
				}

				tags := fm.Tags
				if tags == nil {
					tags = []string{}
				}

				_, err = pageStore.Create(ctx, store.CreateCommand{
					Title:   fm.Title,
					Slug:    slug,
					Project: project,
					Type:    fm.Type,
					Status:  fm.Status,
					Tags:    tags,
					Body:    string(body),
					Author:  fm.DCCreator,
					Message: fmt.Sprintf("migrated from git: %s branch %s", path, branch),
				})
				if err != nil {
					log.Printf("ERROR: %s: %v", path, err)
					errors++
					continue
				}
				migrated++
			}
		}
	}

	log.Printf("migration complete: migrated=%d skipped=%d errors=%d", migrated, skipped, errors)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

Note: `store.SlugFromTitle` needs to be exported. Update `internal/store/store.go`:

```go
// SlugFromTitle converts a title to a URL-friendly slug (exported for migration).
func SlugFromTitle(title string) string {
	return slugFromTitle(title)
}
```

- [ ] **Step 2: Build the migration tool**

Run: `go build -mod=mod ./cmd/prd2wiki-migrate-sqlite/`
Expected: builds without errors

- [ ] **Step 3: Run migration against a copy of the data**

```bash
# SAFETY: copy data first
cp -r data data-pre-migrate-backup
go run -mod=mod ./cmd/prd2wiki-migrate-sqlite/
```

Expected: `migration complete: migrated=298 skipped=N errors=0`

- [ ] **Step 4: Verify migrated data**

```bash
# Quick sanity check
go run -mod=mod -e 'package main; import "database/sql"; import _ "modernc.org/sqlite"; import "fmt"; func main() { db, _ := sql.Open("sqlite", "data/index.db"); var c int; db.QueryRow("SELECT COUNT(*) FROM pages").Scan(&c); fmt.Println("pages:", c); db.QueryRow("SELECT COUNT(*) FROM page_bodies").Scan(&c); fmt.Println("versions:", c) }'
```

- [ ] **Step 5: Commit**

```bash
git add cmd/prd2wiki-migrate-sqlite/ internal/store/store.go
git commit -m "feat(migrate): one-shot git+tree → SQLite migration tool"
```

---

## Task 9: Wire API handlers to `store.PageStore`

This is the big switchover. Each handler stops calling git/tree/librarian for reads and writes, and calls `store` instead.

**Files:**
- Modify: `internal/api/server.go`
- Modify: `internal/api/pages.go`
- Modify: `internal/api/tree_api.go`
- Modify: `internal/api/lifecycle.go`
- Modify: `internal/api/history.go`
- Modify: `internal/api/search.go`
- Modify: `internal/api/auth_test.go`
- Modify: `internal/api/pages_test.go`

This task is large — break into sub-steps per handler group. Each sub-step should compile and have its tests pass before moving to the next.

- [ ] **Step 1: Update `server.go` — replace git/tree fields with store**

Replace the `Server` struct fields:

```go
type ServerConfig struct {
	Addr   string
	Store  *store.PageStore
	Keys   *auth.ServiceKeyStore
	// Librarians kept for validation/intent processing + pippi sync.
	Librarians map[string]*librarian.Librarian
}

type Server struct {
	addr       string
	store      *store.PageStore
	keys       *auth.ServiceKeyStore
	librarians map[string]*librarian.Librarian
}
```

Remove: `repos`, `db`, `indexer`, `search`, `treeHolder`, `blobStore`, `edits`, `migrationAliases`.

Update `Handler()` route registration — keep the same routes but handlers will call `s.store` internally.

- [ ] **Step 2: Update `pages.go` — create/read/update/delete**

`createPage` / `upsertPage`:
```go
func (s *Server) upsertPage(w http.ResponseWriter, r *http.Request, isCreate bool) {
	// ... auth, decode request ...
	if isCreate {
		page, err := s.store.Create(r.Context(), store.CreateCommand{
			Title: req.Title, Project: project, Type: req.Type, Status: req.Status,
			Tags: req.Tags, Body: req.Body, Author: req.Author, Intent: req.Intent,
		})
		// ... write response ...
	} else {
		id := r.PathValue("id")
		page, err := s.store.Update(r.Context(), id, store.UpdateCommand{
			Title: req.Title, Type: req.Type, Status: req.Status,
			Tags: req.Tags, Body: req.Body, Author: req.Author,
		})
		// ... write response ...
	}
}
```

`getPage`:
```go
func (s *Server) getPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	page, err := s.store.GetByUUID(r.Context(), id)
	// ... write response from page fields ...
}
```

`deletePage`:
```go
func (s *Server) deletePage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.Delete(r.Context(), id); err != nil { ... }
	// Notify pippi via librarian.
	if lib, ok := s.librarians[project]; ok {
		lib.RemoveFromIndexes(id)
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 3: Update `tree_api.go` — tree handlers call store**

`treeGetEntry`:
```go
page, err := s.store.GetByTreePath(r.Context(), rest)
```

`treeCreatePage`:
```go
page, err := s.store.Create(r.Context(), store.CreateCommand{
	Title: req.Title, Slug: req.Slug, Project: projectTreePath,
	Type: req.Type, Status: req.Status, Tags: req.Tags,
	Body: req.Body, Author: req.Author,
})
```

`treeUpdatePage`: same pattern with `store.Update`.
`treeDeletePage`: `s.store.Delete`.

- [ ] **Step 4: Update `lifecycle.go`**

`deprecatePage` / `approvePage` / `restorePage` become:
```go
page, err := s.store.Update(r.Context(), id, store.UpdateCommand{
	Status: "deprecated", Author: "system@prd2wiki",
	Message: "deprecate: " + id,
})
```

- [ ] **Step 5: Update `history.go`**

```go
func (s *Server) pageHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	history, err := s.store.History(r.Context(), id, 50)
	// ... format and return ...
}
```

- [ ] **Step 6: Update `search.go`**

```go
func (s *Server) searchPages(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	q := r.URL.Query().Get("q")
	results, err := s.store.Search(r.Context(), project, q, 20)
	// ...
}
```

- [ ] **Step 7: Update test setup**

`setupTestServer` and `setupAuthServer` in test files:
```go
func setupTestServer(t *testing.T) *Server {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	pageStore, err := store.New(dbPath)
	// ...
	return NewServer(ServerConfig{
		Store: pageStore,
		Keys:  keyStore, // if needed
	})
}
```

- [ ] **Step 8: Run all API tests**

Run: `go test ./internal/api/... -v`
Expected: all PASS

- [ ] **Step 9: Commit**

```bash
git add internal/api/
git commit -m "feat(api): wire all handlers to store.PageStore — no more git/tree reads"
```

---

## Task 10: Wire web handlers to `store.PageStore`

**Files:**
- Modify: `internal/web/handler.go`
- Modify: `internal/web/pages.go`
- Modify: `internal/web/list.go`
- Modify: `internal/web/home.go`
- Modify: `internal/web/history.go`
- Modify: `internal/web/nav.go`
- Modify: `internal/web/search.go`

- [ ] **Step 1: Update Handler struct**

Replace `repos`, `treeHolder`, `librarians` with `*store.PageStore`:

```go
type Handler struct {
	store     *store.PageStore
	db        *sql.DB
	templates map[string]*template.Template
	keys      *auth.ServiceKeyStore
	writeToken string
}
```

- [ ] **Step 2: Update page view, edit, list**

`viewPage` reads from `s.store.GetByTreePath` or `s.store.GetByUUID`.
`listPages` reads from `s.store.ListByProject`.
`homePage` reads from `s.store.Projects` + `s.store.ListByProject`.

- [ ] **Step 3: Update nav sidebar**

`preparePageData` builds sidebar from `s.store.Projects()` + `s.store.ListByProject()` instead of tree scanner.

- [ ] **Step 4: Update history/diff views**

Read from `s.store.History` and `s.store.BodyAtVersion`.

- [ ] **Step 5: Run web tests**

Run: `go test ./internal/web/... -v`

- [ ] **Step 6: Commit**

```bash
git add internal/web/
git commit -m "feat(web): wire all web handlers to store.PageStore"
```

---

## Task 11: Wire `app.go` — remove git/tree init, instant startup

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Simplify `app.New()`**

Remove: git repo discovery/init, tree scan, `RebuildFromRepo`, vector index, IndexHolder.

Replace with:
```go
pageStore, err := store.NewFromDB(db)
// ... that's it for storage init
```

Keep: SQLite open, service key store, pippi librarian client, embedder (for pippi sync).

- [ ] **Step 2: Update handler wiring**

```go
webHandler := web.NewHandler(pageStore, db, keys)
apiSrv := api.NewServer(api.ServerConfig{
	Store: pageStore,
	Keys:  keyStore,
	Librarians: librarians,
})
```

- [ ] **Step 3: Build and run**

Run: `go build -mod=mod ./cmd/prd2wiki && ./bin/prd2wiki -config config/prd2wiki.yaml`
Expected: starts in <1 second, serves pages from SQLite.

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): instant startup — SQLite only, no git/tree init"
```

---

## Task 12: Update MCP sidecar

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/client.go`

- [ ] **Step 1: Remove `.link` management from `toolPropose`**

`toolPropose` currently calls project API then writes `.link` client-side. Replace with a call to the tree API (which now calls `store.Create` server-side):

```go
// Instead of client.CreatePage + WriteLinkFileAtTreeURL,
// just call the tree API endpoint which handles everything.
```

- [ ] **Step 2: Run MCP tests**

Run: `go test ./internal/mcp/... -v`

- [ ] **Step 3: Commit**

```bash
git add internal/mcp/
git commit -m "feat(mcp): drop .link workaround — server handles everything"
```

---

## Task 13: Attachments storage decision

Attachments are currently stored in git (`pages/{id}/_attachments/{filename}`). With git removed, they need a new home.

**Options:**
- (a) Store in SQLite as BLOBs (simple, but large files bloat the DB)
- (b) Store on filesystem at `data/attachments/{page_uuid}/{filename}` (simple, fast)
- (c) Use the existing blob store (`internal/blob`)

**Recommendation:** (b) — filesystem for binary files, SQL for metadata. Add an `attachments` table:

```sql
CREATE TABLE IF NOT EXISTS attachments (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    page_uuid TEXT NOT NULL,
    filename  TEXT NOT NULL,
    path      TEXT NOT NULL,
    size      INTEGER NOT NULL,
    mime_type TEXT NOT NULL,
    author    TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(page_uuid, filename)
);
```

- [ ] **Step 1: Add attachments table to schema**
- [ ] **Step 2: Update `attachments.go` handlers**
- [ ] **Step 3: Test upload/download/list**
- [ ] **Step 4: Commit**

---

## Task 14: Remove dead code

**Files to delete:**
- `internal/git/*.go` (entire package)
- `internal/tree/*.go` (entire package)
- `internal/pagepath/*.go` (entire package)
- `internal/api/projects_redirect.go`
- `internal/api/tree_multipart.go` + test

**Files to clean:**
- `internal/librarian/librarian.go` — remove `repo` field, `readPageBodyAcrossBranches`, git-dependent code
- `internal/index/indexer.go` — remove `RebuildFromRepo` (FTS now maintained by store)
- `go.mod` — remove `go-git` dependencies if no longer imported

- [ ] **Step 1: Delete packages**

```bash
rm -rf internal/git internal/tree internal/pagepath
rm internal/api/projects_redirect.go internal/api/tree_multipart.go internal/api/tree_multipart_test.go
```

- [ ] **Step 2: Clean librarian**

Remove git dependency from `Librarian` struct and methods.

- [ ] **Step 3: Clean index**

Remove `RebuildFromRepo` and related imports.

- [ ] **Step 4: Build — expect clean compile**

Run: `go build -mod=mod ./...`

- [ ] **Step 5: Run all tests**

Run: `go test -mod=mod ./... -count=1`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "chore: remove git/tree/link infrastructure — SQLite is the sole storage engine"
```

---

## Task 15: Smoke test and report

- [ ] **Step 1: Build and restart**

```bash
go build -mod=mod -o bin/prd2wiki ./cmd/prd2wiki
# kill old process, start new
```

- [ ] **Step 2: Verify startup is instant**

Check logs — no "rebuilding index" messages.

- [ ] **Step 3: Run smoke tests**

```bash
# Read
curl -sS http://192.168.22.56:8082/api/tree/prd2wiki/agent-wiki-access-and-mcp-runbook | head -1

# Write with auth
curl -sS -X POST .../api/tree/prd2wiki/pages -H "Authorization: Bearer ..." -d '{"title":"Smoke","body":"test"}'

# Delete
curl -sS -X DELETE .../api/tree/prd2wiki/smoke-test-slug -H "Authorization: Bearer ..."

# Search
curl -sS .../api/projects/prd2wiki/search?q=auth

# History
curl -sS .../api/tree/prd2wiki/agent-wiki-access-and-mcp-runbook/history
```

- [ ] **Step 4: Update report on wiki**

- [ ] **Step 5: Final commit**

```bash
git commit --allow-empty -m "Phase 3 complete: SQLite-only storage, git/tree/link removed"
```

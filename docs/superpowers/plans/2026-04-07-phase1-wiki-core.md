# Phase 1: Wiki Core — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the foundational wiki core — a Go binary that manages git-backed markdown pages with YAML frontmatter, schema validation, SQLite computed index, and a REST API for CRUD, search, provenance queries, and branch operations.

**Architecture:** Single Go binary using `net/http` stdlib. Content stored as markdown files in git bare repos (one per project) via `go-git`. SQLite computed index (FTS5, metadata, provenance graph) rebuilt from git state. Librarian and vector index are Phase 2 — this phase delivers a working wiki with keyword search and schema validation.

**Tech Stack:** Go 1.24+, `golang.org/x/*`, `go-git/go-git/v5`, `yuin/goldmark`, `modernc.org/sqlite` (pure Go), `net/http` stdlib

**Spec Reference:** `/srv/prd2wiki/docs/superpowers/specs/2026-04-07-prd2wiki-design-04.md`

**Prior Art:**
- Gitea wiki module pattern (git bare repo per wiki, service layer design)
- Pippi `internal/lancedb/sqlite.go` (WAL mode, migrations) — adapt for our schema

---

## File Structure

```
prd2wiki/
├── cmd/
│   └── prd2wiki/
│       └── main.go                    # Binary entrypoint, wires everything
├── internal/
│   ├── git/
│   │   ├── repo.go                    # Git bare repo management (init, read, write, branch)
│   │   ├── repo_test.go
│   │   ├── page.go                    # Page read/write operations (markdown + frontmatter)
│   │   └── page_test.go
│   ├── schema/
│   │   ├── frontmatter.go             # YAML frontmatter types and parsing
│   │   ├── frontmatter_test.go
│   │   ├── validate.go                # Schema validation per document type
│   │   └── validate_test.go
│   ├── index/
│   │   ├── sqlite.go                  # SQLite setup, WAL, migrations
│   │   ├── sqlite_test.go
│   │   ├── indexer.go                 # Build/rebuild index from git state
│   │   ├── indexer_test.go
│   │   ├── search.go                  # FTS5 + metadata queries
│   │   └── search_test.go
│   ├── api/
│   │   ├── server.go                  # HTTP server setup, middleware, routing
│   │   ├── pages.go                   # Page CRUD handlers
│   │   ├── pages_test.go
│   │   ├── search.go                  # Search endpoint handlers
│   │   ├── search_test.go
│   │   ├── branches.go                # Branch operations handlers
│   │   ├── branches_test.go
│   │   ├── references.go              # Provenance/reference tree handlers
│   │   └── references_test.go
│   └── wiki/
│       ├── wiki.go                    # Top-level Wiki type, orchestrates git+index+api
│       └── wiki_test.go
├── config/
│   ├── projects.yaml                  # Project definitions and RBAC
│   └── prd2wiki.yaml                  # Server config (port, data dir, etc.)
├── go.mod
├── go.sum
└── Makefile
```

---

### Task 1: Project Scaffolding

**Files:**
- Create: `cmd/prd2wiki/main.go`
- Create: `go.mod`
- Create: `Makefile`
- Create: `config/prd2wiki.yaml`

- [ ] **Step 1: Initialize Go module**

```bash
cd /srv/prd2wiki
go mod init github.com/frodex/prd2wiki
```

- [ ] **Step 2: Create Makefile**

```makefile
.PHONY: build test run clean

build:
	go build -o bin/prd2wiki ./cmd/prd2wiki

test:
	go test ./... -v -count=1

run: build
	./bin/prd2wiki -config config/prd2wiki.yaml

clean:
	rm -rf bin/
```

- [ ] **Step 3: Create default config**

```yaml
# config/prd2wiki.yaml
server:
  addr: ":8080"
  
data:
  dir: "./data"           # git repos and indexes stored here

logging:
  level: "info"
```

- [ ] **Step 4: Create minimal main.go**

```go
// cmd/prd2wiki/main.go
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	configPath := flag.String("config", "config/prd2wiki.yaml", "path to config file")
	flag.Parse()

	fmt.Printf("prd2wiki starting with config: %s\n", *configPath)
	os.Exit(0)
}
```

- [ ] **Step 5: Verify build**

Run: `make build`
Expected: Binary at `bin/prd2wiki`, exits cleanly

- [ ] **Step 6: Commit**

```bash
git init
echo "bin/" > .gitignore
echo "data/" >> .gitignore
git add go.mod cmd/ Makefile config/ .gitignore
git commit -m "feat: project scaffolding — Go module, Makefile, config, entrypoint"
```

---

### Task 2: YAML Frontmatter Parsing

**Files:**
- Create: `internal/schema/frontmatter.go`
- Create: `internal/schema/frontmatter_test.go`

- [ ] **Step 1: Write failing test for frontmatter parsing**

```go
// internal/schema/frontmatter_test.go
package schema

import (
	"testing"
	"time"
)

func TestParseFrontmatter(t *testing.T) {
	raw := `---
id: PRD-042
title: "Authentication Requirements"
type: requirement
status: active
dc.creator: "jane@example.com"
dc.created: 2026-03-15
dc.modified: 2026-04-01
trust_level: 3
conformance: valid
tags: [authentication, security]
provenance:
  sources:
    - ref: "wiki://project-a/auth-research"
      version: 3
      checksum: "sha256:abc123"
      retrieved: 2026-03-20
      status: valid
  contributors:
    - identity: "jane@example.com"
      role: author
supersedes: PRD-031
updates: [PRD-028, PRD-033]
---
# Page content here`

	fm, body, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if fm.ID != "PRD-042" {
		t.Errorf("ID = %q, want PRD-042", fm.ID)
	}
	if fm.Type != "requirement" {
		t.Errorf("Type = %q, want requirement", fm.Type)
	}
	if fm.Status != "active" {
		t.Errorf("Status = %q, want active", fm.Status)
	}
	if fm.TrustLevel != 3 {
		t.Errorf("TrustLevel = %d, want 3", fm.TrustLevel)
	}
	if fm.DCCreator != "jane@example.com" {
		t.Errorf("DCCreator = %q, want jane@example.com", fm.DCCreator)
	}
	if len(fm.Tags) != 2 {
		t.Errorf("Tags len = %d, want 2", len(fm.Tags))
	}
	if len(fm.Provenance.Sources) != 1 {
		t.Errorf("Sources len = %d, want 1", len(fm.Provenance.Sources))
	}
	if fm.Provenance.Sources[0].Ref != "wiki://project-a/auth-research" {
		t.Errorf("Source ref = %q", fm.Provenance.Sources[0].Ref)
	}
	if fm.Supersedes != "PRD-031" {
		t.Errorf("Supersedes = %q, want PRD-031", fm.Supersedes)
	}
	if len(fm.Updates) != 2 {
		t.Errorf("Updates len = %d, want 2", len(fm.Updates))
	}
	if string(body) != "# Page content here" {
		t.Errorf("Body = %q", string(body))
	}
}

func TestSerializeFrontmatter(t *testing.T) {
	fm := &Frontmatter{
		ID:     "PRD-001",
		Title:  "Test Page",
		Type:   "concept",
		Status: "draft",
	}

	data, err := Serialize(fm, []byte("# Content"))
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Round-trip
	fm2, body, err := Parse(data)
	if err != nil {
		t.Fatalf("Round-trip parse failed: %v", err)
	}
	if fm2.ID != "PRD-001" {
		t.Errorf("Round-trip ID = %q", fm2.ID)
	}
	if string(body) != "# Content" {
		t.Errorf("Round-trip body = %q", string(body))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/schema/ -v -run TestParse`
Expected: FAIL — package doesn't exist yet

- [ ] **Step 3: Implement frontmatter types and parser**

```go
// internal/schema/frontmatter.go
package schema

import (
	"bytes"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Frontmatter represents the YAML frontmatter of a wiki page.
// Field names use Dublin Core (dc.*), W3C PROV-DM (provenance.*),
// RFC lifecycle (supersedes/updates), and SLSA-adapted trust levels.
type Frontmatter struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Type        string `yaml:"type"`
	Status      string `yaml:"status"`
	DCCreator   string `yaml:"dc.creator,omitempty"`
	DCCreated   Date   `yaml:"dc.created,omitempty"`
	DCModified  Date   `yaml:"dc.modified,omitempty"`
	DCRights    string `yaml:"dc.rights,omitempty"`
	TrustLevel  int    `yaml:"trust_level,omitempty"`
	Conformance string `yaml:"conformance,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`

	Provenance   Provenance `yaml:"provenance,omitempty"`
	Supersedes   string     `yaml:"supersedes,omitempty"`
	SupersededBy string     `yaml:"superseded_by,omitempty"`
	Updates      []string   `yaml:"updates,omitempty"`

	// Source-specific metadata (type: source pages)
	SourceMeta *SourceMeta `yaml:"source_meta,omitempty"`

	// Access overrides (optional)
	Access *Access `yaml:"access,omitempty"`

	// Challenge tracking
	ContestedBy string `yaml:"contested_by,omitempty"`
}

type Provenance struct {
	Sources      []Source      `yaml:"sources,omitempty"`
	Contributors []Contributor `yaml:"contributors,omitempty"`
}

type Source struct {
	Ref       string `yaml:"ref"`
	Title     string `yaml:"title,omitempty"`
	Version   int    `yaml:"version,omitempty"`
	Checksum  string `yaml:"checksum,omitempty"`
	Retrieved Date   `yaml:"retrieved,omitempty"`
	Status    string `yaml:"status,omitempty"`
}

type Contributor struct {
	Identity string `yaml:"identity"`
	Role     string `yaml:"role"`
	Decision string `yaml:"decision,omitempty"`
	Date     Date   `yaml:"date,omitempty"`
}

type SourceMeta struct {
	URL       string `yaml:"url,omitempty"`
	Kind      string `yaml:"kind,omitempty"`
	Authority string `yaml:"authority,omitempty"`
	Retrieved Date   `yaml:"retrieved,omitempty"`
}

type Access struct {
	RestrictTo []string `yaml:"restrict_to,omitempty"`
}

// Date wraps time.Time for flexible YAML date parsing.
type Date struct {
	time.Time
}

func (d *Date) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			d.Time = t
			return nil
		}
	}
	return fmt.Errorf("cannot parse date: %q", s)
}

func (d Date) MarshalYAML() (interface{}, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.Format("2006-01-02"), nil
}

var frontmatterSep = []byte("---")

// Parse splits a markdown file into frontmatter and body.
func Parse(data []byte) (*Frontmatter, []byte, error) {
	data = bytes.TrimSpace(data)
	if !bytes.HasPrefix(data, frontmatterSep) {
		return nil, data, nil
	}

	rest := data[len(frontmatterSep):]
	end := bytes.Index(rest, frontmatterSep)
	if end < 0 {
		return nil, data, fmt.Errorf("unterminated frontmatter")
	}

	fmRaw := rest[:end]
	body := bytes.TrimSpace(rest[end+len(frontmatterSep):])

	var fm Frontmatter
	if err := yaml.Unmarshal(fmRaw, &fm); err != nil {
		return nil, nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	return &fm, body, nil
}

// Serialize combines frontmatter and body into a markdown file.
func Serialize(fm *Frontmatter, body []byte) ([]byte, error) {
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}

	var buf bytes.Buffer
	buf.Write(frontmatterSep)
	buf.WriteByte('\n')
	buf.Write(fmBytes)
	buf.Write(frontmatterSep)
	buf.WriteByte('\n')
	buf.Write(body)
	buf.WriteByte('\n')

	return buf.Bytes(), nil
}
```

- [ ] **Step 4: Add yaml dependency**

```bash
go get gopkg.in/yaml.v3
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/schema/ -v`
Expected: PASS — both TestParseFrontmatter and TestSerializeFrontmatter

- [ ] **Step 6: Commit**

```bash
git add internal/schema/ go.mod go.sum
git commit -m "feat: YAML frontmatter parsing with Dublin Core, PROV-DM, and trust level types"
```

---

### Task 3: Schema Validation

**Files:**
- Create: `internal/schema/validate.go`
- Create: `internal/schema/validate_test.go`

- [ ] **Step 1: Write failing tests for validation**

```go
// internal/schema/validate_test.go
package schema

import "testing"

func TestValidateRequirement(t *testing.T) {
	fm := &Frontmatter{
		ID:     "PRD-042",
		Title:  "Auth Requirements",
		Type:   "requirement",
		Status: "active",
	}

	issues := Validate(fm)

	// No errors for minimal valid requirement
	errors := filterSeverity(issues, SeverityError)
	if len(errors) > 0 {
		t.Errorf("unexpected errors: %v", errors)
	}
}

func TestValidateMissingRequired(t *testing.T) {
	fm := &Frontmatter{
		Title: "No ID or Type",
	}

	issues := Validate(fm)
	errors := filterSeverity(issues, SeverityError)

	if len(errors) < 2 {
		t.Errorf("expected at least 2 errors (missing id, type), got %d", len(errors))
	}
}

func TestValidateInvalidType(t *testing.T) {
	fm := &Frontmatter{
		ID:   "X-001",
		Title: "Bad Type",
		Type: "nonexistent",
	}

	issues := Validate(fm)
	errors := filterSeverity(issues, SeverityError)

	found := false
	for _, e := range errors {
		if e.Field == "type" {
			found = true
		}
	}
	if !found {
		t.Error("expected error on type field")
	}
}

func TestValidateInvalidStatus(t *testing.T) {
	fm := &Frontmatter{
		ID:     "X-001",
		Title:  "Bad Status",
		Type:   "concept",
		Status: "bogus",
	}

	issues := Validate(fm)
	errors := filterSeverity(issues, SeverityError)

	found := false
	for _, e := range errors {
		if e.Field == "status" {
			found = true
		}
	}
	if !found {
		t.Error("expected error on status field")
	}
}

func TestValidateWarningMissingProvenance(t *testing.T) {
	fm := &Frontmatter{
		ID:     "PRD-042",
		Title:  "No Sources",
		Type:   "requirement",
		Status: "active",
	}

	issues := Validate(fm)
	warnings := filterSeverity(issues, SeverityWarning)

	if len(warnings) == 0 {
		t.Error("expected warning about missing provenance sources")
	}
}

func filterSeverity(issues []Issue, sev string) []Issue {
	var out []Issue
	for _, i := range issues {
		if i.Severity == sev {
			out = append(out, i)
		}
	}
	return out
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/schema/ -v -run TestValidate`
Expected: FAIL — Validate function doesn't exist

- [ ] **Step 3: Implement schema validation**

```go
// internal/schema/validate.go
package schema

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

type Issue struct {
	Severity string `json:"severity"`
	Field    string `json:"field"`
	Message  string `json:"message"`
}

var validTypes = map[string]bool{
	"requirement": true,
	"concept":     true,
	"task":        true,
	"reference":   true,
	"decision":    true,
	"source":      true,
	"config":      true,
}

var validStatuses = map[string]bool{
	"draft":      true,
	"review":     true,
	"active":     true,
	"contested":  true,
	"stale":      true,
	"superseded": true,
	"deprecated": true,
}

// Validate checks a Frontmatter against the schema rules.
// Returns issues sorted by severity (errors first).
func Validate(fm *Frontmatter) []Issue {
	var issues []Issue

	if fm.ID == "" {
		issues = append(issues, Issue{SeverityError, "id", "id is required"})
	}
	if fm.Title == "" {
		issues = append(issues, Issue{SeverityError, "title", "title is required"})
	}
	if fm.Type == "" {
		issues = append(issues, Issue{SeverityError, "type", "type is required"})
	} else if !validTypes[fm.Type] {
		issues = append(issues, Issue{SeverityError, "type", "invalid type: " + fm.Type})
	}
	if fm.Status != "" && !validStatuses[fm.Status] {
		issues = append(issues, Issue{SeverityError, "status", "invalid status: " + fm.Status})
	}

	if fm.TrustLevel < 0 || fm.TrustLevel > 3 {
		issues = append(issues, Issue{SeverityError, "trust_level", "trust_level must be 0-3"})
	}

	// Warnings
	if len(fm.Provenance.Sources) == 0 {
		issues = append(issues, Issue{SeverityWarning, "provenance.sources", "no provenance sources listed"})
	}
	if fm.DCCreator == "" {
		issues = append(issues, Issue{SeverityWarning, "dc.creator", "no creator specified"})
	}

	// Source-type specific validation
	if fm.Type == "source" && fm.SourceMeta == nil {
		issues = append(issues, Issue{SeverityError, "source_meta", "source_meta required for type: source"})
	}

	return issues
}

// HasErrors returns true if any issue has severity "error".
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/schema/ -v`
Expected: PASS — all validation tests

- [ ] **Step 5: Commit**

```bash
git add internal/schema/validate.go internal/schema/validate_test.go
git commit -m "feat: schema validation with error/warning/info severity levels"
```

---

### Task 4: Git Bare Repo Management

**Files:**
- Create: `internal/git/repo.go`
- Create: `internal/git/repo_test.go`

- [ ] **Step 1: Write failing tests for git operations**

```go
// internal/git/repo_test.go
package git

import (
	"os"
	"testing"
)

func TestInitRepo(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "test-project")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}
	if repo == nil {
		t.Fatal("repo is nil")
	}

	// Should be a bare repo
	repoPath := dir + "/test-project.wiki.git"
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		t.Errorf("bare repo not created at %s", repoPath)
	}
}

func TestOpenRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := InitRepo(dir, "test-project")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	repo, err := OpenRepo(dir, "test-project")
	if err != nil {
		t.Fatalf("OpenRepo failed: %v", err)
	}
	if repo == nil {
		t.Fatal("repo is nil")
	}
}

func TestListBranches(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "test-project")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	// Write initial page to create the truth branch
	err = repo.WritePage("truth", "pages/test.md", []byte("---\nid: test\n---\n# Test"), "init", "test@example.com")
	if err != nil {
		t.Fatalf("WritePage failed: %v", err)
	}

	branches, err := repo.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}
	if len(branches) == 0 {
		t.Error("expected at least one branch")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/git/ -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Add go-git dependency**

```bash
go get github.com/go-git/go-git/v5
```

- [ ] **Step 4: Implement git repo management**

```go
// internal/git/repo.go
package git

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/plumbing/cache"

	"github.com/go-billy/v5/osfs"
	"github.com/go-billy/v5/memfs"
)

// Repo wraps a git bare repository for wiki operations.
type Repo struct {
	repo    *gogit.Repository
	path    string
	project string
	mu      sync.Mutex // serialize writes (Gitea pattern)
}

// InitRepo creates a new bare git repository for a project.
func InitRepo(dataDir, project string) (*Repo, error) {
	repoPath := filepath.Join(dataDir, project+".wiki.git")

	fs := osfs.New(repoPath)
	stor := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())

	repo, err := gogit.Init(stor, nil) // nil worktree = bare
	if err != nil {
		return nil, fmt.Errorf("init bare repo: %w", err)
	}

	return &Repo{repo: repo, path: repoPath, project: project}, nil
}

// OpenRepo opens an existing bare git repository.
func OpenRepo(dataDir, project string) (*Repo, error) {
	repoPath := filepath.Join(dataDir, project+".wiki.git")

	fs := osfs.New(repoPath)
	stor := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())

	repo, err := gogit.Open(stor, nil)
	if err != nil {
		return nil, fmt.Errorf("open bare repo: %w", err)
	}

	return &Repo{repo: repo, path: repoPath, project: project}, nil
}

// WritePage writes a file to a branch, creating the branch if needed.
func (r *Repo) WritePage(branch, path string, content []byte, message, author string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	refName := plumbing.NewBranchReferenceName(branch)

	// Get parent commit if branch exists
	var parentHash *plumbing.Hash
	ref, err := r.repo.Reference(refName, true)
	if err == nil {
		h := ref.Hash()
		parentHash = &h
	}

	// Build tree with the new/updated file
	tree, err := r.buildTree(parentHash, path, content)
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}

	// Create commit
	sig := &object.Signature{
		Name:  author,
		Email: author,
		When:  time.Now(),
	}

	commit := &object.Commit{
		Author:    *sig,
		Committer: *sig,
		Message:   message,
		TreeHash:  tree,
	}

	if parentHash != nil {
		commit.ParentHashes = []plumbing.Hash{*parentHash}
	}

	obj := r.repo.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		return fmt.Errorf("encode commit: %w", err)
	}
	commitHash, err := r.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return fmt.Errorf("store commit: %w", err)
	}

	// Update branch reference
	newRef := plumbing.NewHashReference(refName, commitHash)
	return r.repo.Storer.SetReference(newRef)
}

// ReadPage reads a file from a branch.
func (r *Repo) ReadPage(branch, path string) ([]byte, error) {
	refName := plumbing.NewBranchReferenceName(branch)
	ref, err := r.repo.Reference(refName, true)
	if err != nil {
		return nil, fmt.Errorf("branch %q not found: %w", branch, err)
	}

	commit, err := r.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("read commit: %w", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("read tree: %w", err)
	}

	file, err := tree.File(path)
	if err != nil {
		return nil, fmt.Errorf("file %q not found: %w", path, err)
	}

	content, err := file.Contents()
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return []byte(content), nil
}

// ListBranches returns all branch names.
func (r *Repo) ListBranches() ([]string, error) {
	refs, err := r.repo.References()
	if err != nil {
		return nil, err
	}

	var branches []string
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsBranch() {
			branches = append(branches, ref.Name().Short())
		}
		return nil
	})
	return branches, err
}

// ListPages returns all file paths on a branch.
func (r *Repo) ListPages(branch string) ([]string, error) {
	refName := plumbing.NewBranchReferenceName(branch)
	ref, err := r.repo.Reference(refName, true)
	if err != nil {
		return nil, fmt.Errorf("branch %q not found: %w", branch, err)
	}

	commit, err := r.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	var paths []string
	err = tree.Files().ForEach(func(f *object.File) error {
		paths = append(paths, f.Name)
		return nil
	})
	return paths, err
}

// buildTree creates a tree object with the given file added/updated.
func (r *Repo) buildTree(parentHash *plumbing.Hash, path string, content []byte) (plumbing.Hash, error) {
	// Store blob
	blob := r.repo.Storer.NewEncodedObject()
	blob.SetType(plumbing.BlobObject)
	w, err := blob.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if _, err := w.Write(content); err != nil {
		return plumbing.ZeroHash, err
	}
	if err := w.Close(); err != nil {
		return plumbing.ZeroHash, err
	}
	blobHash, err := r.repo.Storer.SetEncodedObject(blob)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	// Build tree entries from parent + new file
	entries := []object.TreeEntry{}

	if parentHash != nil {
		parentCommit, err := r.repo.CommitObject(*parentHash)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		parentTree, err := parentCommit.Tree()
		if err != nil {
			return plumbing.ZeroHash, err
		}
		for _, entry := range parentTree.Entries {
			if entry.Name != path {
				entries = append(entries, entry)
			}
		}
	}

	entries = append(entries, object.TreeEntry{
		Name: path,
		Mode: 0100644,
		Hash: blobHash,
	})

	// Store tree
	treeObj := &object.Tree{Entries: entries}
	obj := r.repo.Storer.NewEncodedObject()
	if err := treeObj.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return r.repo.Storer.SetEncodedObject(obj)
}
```

- [ ] **Step 5: Add go-billy dependency**

```bash
go get github.com/go-billy/v5
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/git/ -v`
Expected: PASS — init, open, write, list branches all work

- [ ] **Step 7: Commit**

```bash
git add internal/git/ go.mod go.sum
git commit -m "feat: git bare repo management — init, read, write, branch, list"
```

---

### Task 5: Page Operations (Read/Write with Frontmatter)

**Files:**
- Create: `internal/git/page.go`
- Create: `internal/git/page_test.go`

- [ ] **Step 1: Write failing test for page operations**

```go
// internal/git/page_test.go
package git

import (
	"testing"

	"github.com/frodex/prd2wiki/internal/schema"
)

func TestWriteAndReadPage(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "test-project")
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	fm := &schema.Frontmatter{
		ID:     "PRD-001",
		Title:  "Test Page",
		Type:   "concept",
		Status: "draft",
	}

	err = repo.WritePageWithMeta("truth", "pages/test.md", fm, []byte("# Hello World"), "create test page", "test@example.com")
	if err != nil {
		t.Fatalf("WritePageWithMeta: %v", err)
	}

	readFM, body, err := repo.ReadPageWithMeta("truth", "pages/test.md")
	if err != nil {
		t.Fatalf("ReadPageWithMeta: %v", err)
	}

	if readFM.ID != "PRD-001" {
		t.Errorf("ID = %q, want PRD-001", readFM.ID)
	}
	if readFM.Type != "concept" {
		t.Errorf("Type = %q, want concept", readFM.Type)
	}
	if string(body) != "# Hello World\n" {
		t.Errorf("Body = %q", string(body))
	}
}

func TestDeletePage(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "test-project")
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// Create a page
	fm := &schema.Frontmatter{ID: "DEL-001", Title: "Delete Me", Type: "concept", Status: "draft"}
	err = repo.WritePageWithMeta("truth", "pages/delete-me.md", fm, []byte("# Gone"), "create", "test@example.com")
	if err != nil {
		t.Fatalf("WritePageWithMeta: %v", err)
	}

	// Delete it
	err = repo.DeletePage("truth", "pages/delete-me.md", "delete page", "test@example.com")
	if err != nil {
		t.Fatalf("DeletePage: %v", err)
	}

	// Should not be readable
	_, _, err = repo.ReadPageWithMeta("truth", "pages/delete-me.md")
	if err == nil {
		t.Error("expected error reading deleted page")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/git/ -v -run TestWriteAndRead`
Expected: FAIL — WritePageWithMeta doesn't exist

- [ ] **Step 3: Implement page operations**

```go
// internal/git/page.go
package git

import (
	"fmt"

	"github.com/frodex/prd2wiki/internal/schema"
)

// WritePageWithMeta serializes frontmatter + body and writes to git.
func (r *Repo) WritePageWithMeta(branch, path string, fm *schema.Frontmatter, body []byte, message, author string) error {
	data, err := schema.Serialize(fm, body)
	if err != nil {
		return fmt.Errorf("serialize page: %w", err)
	}
	return r.WritePage(branch, path, data, message, author)
}

// ReadPageWithMeta reads a page and parses its frontmatter.
func (r *Repo) ReadPageWithMeta(branch, path string) (*schema.Frontmatter, []byte, error) {
	data, err := r.ReadPage(branch, path)
	if err != nil {
		return nil, nil, err
	}

	fm, body, err := schema.Parse(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	return fm, body, nil
}

// DeletePage removes a file from a branch.
func (r *Repo) DeletePage(branch, path string, message, author string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	refName := plumbing.NewBranchReferenceName(branch)
	ref, err := r.repo.Reference(refName, true)
	if err != nil {
		return fmt.Errorf("branch %q not found: %w", branch, err)
	}

	commit, err := r.repo.CommitObject(ref.Hash())
	if err != nil {
		return err
	}
	parentTree, err := commit.Tree()
	if err != nil {
		return err
	}

	// Rebuild tree without the deleted file
	var entries []object.TreeEntry
	for _, entry := range parentTree.Entries {
		if entry.Name != path {
			entries = append(entries, entry)
		}
	}

	treeObj := &object.Tree{Entries: entries}
	obj := r.repo.Storer.NewEncodedObject()
	if err := treeObj.Encode(obj); err != nil {
		return err
	}
	treeHash, err := r.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return err
	}

	// Create commit
	sig := &object.Signature{
		Name:  author,
		Email: author,
		When:  time.Now(),
	}
	delCommit := &object.Commit{
		Author:       *sig,
		Committer:    *sig,
		Message:      message,
		TreeHash:     treeHash,
		ParentHashes: []plumbing.Hash{ref.Hash()},
	}
	cObj := r.repo.Storer.NewEncodedObject()
	if err := delCommit.Encode(cObj); err != nil {
		return err
	}
	commitHash, err := r.repo.Storer.SetEncodedObject(cObj)
	if err != nil {
		return err
	}

	newRef := plumbing.NewHashReference(refName, commitHash)
	return r.repo.Storer.SetReference(newRef)
}
```

- [ ] **Step 4: Add missing imports to page.go**

Add `"time"` and the go-git plumbing/object imports to `page.go`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/git/ -v`
Expected: PASS — all git tests including page operations

- [ ] **Step 6: Commit**

```bash
git add internal/git/page.go internal/git/page_test.go
git commit -m "feat: page operations — read/write with frontmatter, delete"
```

---

### Task 6: SQLite Index — Setup and Migrations

**Files:**
- Create: `internal/index/sqlite.go`
- Create: `internal/index/sqlite_test.go`

- [ ] **Step 1: Write failing test for SQLite setup**

```go
// internal/index/sqlite_test.go
package index

import (
	"path/filepath"
	"testing"
)

func TestOpenDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	db, err := OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("OpenDatabase: %v", err)
	}
	defer db.Close()

	// Verify WAL mode
	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want wal", journalMode)
	}
}

func TestMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	db, err := OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("OpenDatabase: %v", err)
	}
	defer db.Close()

	// pages table should exist
	var count int
	err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='pages'").Scan(&count)
	if err != nil {
		t.Fatalf("check pages table: %v", err)
	}
	if count != 1 {
		t.Error("pages table not created")
	}

	// FTS table should exist
	err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='pages_fts'").Scan(&count)
	if err != nil {
		t.Fatalf("check FTS table: %v", err)
	}
	if count != 1 {
		t.Error("pages_fts table not created")
	}

	// provenance_edges table should exist
	err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='provenance_edges'").Scan(&count)
	if err != nil {
		t.Fatalf("check provenance_edges table: %v", err)
	}
	if count != 1 {
		t.Error("provenance_edges table not created")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/index/ -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Add SQLite dependency**

```bash
go get modernc.org/sqlite
```

- [ ] **Step 4: Implement SQLite setup with migrations**

```go
// internal/index/sqlite.go
package index

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// OpenDatabase opens a SQLite database with WAL mode and runs migrations.
func OpenDatabase(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=wal&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	// Force WAL mode
	if _, err := db.Exec("PRAGMA journal_mode=wal"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS pages (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'draft',
			path TEXT NOT NULL,
			project TEXT NOT NULL,
			branch TEXT NOT NULL DEFAULT 'truth',
			trust_level INTEGER DEFAULT 0,
			conformance TEXT DEFAULT 'pending',
			dc_creator TEXT,
			dc_created TEXT,
			dc_modified TEXT,
			supersedes TEXT,
			superseded_by TEXT,
			contested_by TEXT,
			tags TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS pages_fts USING fts5(
			id,
			title,
			body,
			tags,
			content='pages',
			content_rowid='rowid'
		)`,
		`CREATE TABLE IF NOT EXISTS provenance_edges (
			source_page TEXT NOT NULL,
			target_ref TEXT NOT NULL,
			target_version INTEGER,
			target_checksum TEXT,
			status TEXT DEFAULT 'valid',
			PRIMARY KEY (source_page, target_ref)
		)`,
		`CREATE TABLE IF NOT EXISTS vocabulary (
			term TEXT PRIMARY KEY,
			category TEXT NOT NULL,
			usage_count INTEGER DEFAULT 1,
			canonical INTEGER DEFAULT 1,
			aliases TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pages_project ON pages(project)`,
		`CREATE INDEX IF NOT EXISTS idx_pages_type ON pages(type)`,
		`CREATE INDEX IF NOT EXISTS idx_pages_status ON pages(status)`,
		`CREATE INDEX IF NOT EXISTS idx_provenance_target ON provenance_edges(target_ref)`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %s: %w", m[:50], err)
		}
	}

	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/index/ -v`
Expected: PASS — database opens with WAL, all tables created

- [ ] **Step 6: Commit**

```bash
git add internal/index/ go.mod go.sum
git commit -m "feat: SQLite computed index — WAL mode, migrations, FTS5, provenance edges"
```

---

### Task 7: Index Builder — Populate Index from Git

**Files:**
- Create: `internal/index/indexer.go`
- Create: `internal/index/indexer_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/index/indexer_test.go
package index

import (
	"path/filepath"
	"testing"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/schema"
)

func TestIndexPage(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	db, err := OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("OpenDatabase: %v", err)
	}
	defer db.Close()

	indexer := NewIndexer(db)

	fm := &schema.Frontmatter{
		ID:     "PRD-001",
		Title:  "Test Page",
		Type:   "requirement",
		Status: "active",
		Tags:   []string{"auth", "security"},
		Provenance: schema.Provenance{
			Sources: []schema.Source{
				{Ref: "wiki://project-a/research", Version: 2, Checksum: "sha256:abc"},
			},
		},
	}

	err = indexer.IndexPage("project-a", "truth", "pages/test.md", fm, []byte("# Hello World"))
	if err != nil {
		t.Fatalf("IndexPage: %v", err)
	}

	// Verify page is in the index
	var title string
	err = db.QueryRow("SELECT title FROM pages WHERE id = ?", "PRD-001").Scan(&title)
	if err != nil {
		t.Fatalf("query page: %v", err)
	}
	if title != "Test Page" {
		t.Errorf("title = %q", title)
	}

	// Verify provenance edge
	var targetRef string
	err = db.QueryRow("SELECT target_ref FROM provenance_edges WHERE source_page = ?", "PRD-001").Scan(&targetRef)
	if err != nil {
		t.Fatalf("query provenance: %v", err)
	}
	if targetRef != "wiki://project-a/research" {
		t.Errorf("target_ref = %q", targetRef)
	}
}

func TestRebuildIndex(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	db, err := OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("OpenDatabase: %v", err)
	}
	defer db.Close()

	// Create a repo with a page
	repo, err := wgit.InitRepo(dir, "test-project")
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	fm := &schema.Frontmatter{ID: "RB-001", Title: "Rebuild Test", Type: "concept", Status: "draft"}
	err = repo.WritePageWithMeta("truth", "pages/rebuild.md", fm, []byte("# Rebuild"), "init", "test@example.com")
	if err != nil {
		t.Fatalf("WritePageWithMeta: %v", err)
	}

	indexer := NewIndexer(db)
	err = indexer.RebuildFromRepo("test-project", repo, "truth")
	if err != nil {
		t.Fatalf("RebuildFromRepo: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT count(*) FROM pages WHERE project = ?", "test-project").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/index/ -v -run TestIndex`
Expected: FAIL — NewIndexer doesn't exist

- [ ] **Step 3: Implement indexer**

```go
// internal/index/indexer.go
package index

import (
	"database/sql"
	"fmt"
	"strings"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/schema"
)

type Indexer struct {
	db *sql.DB
}

func NewIndexer(db *sql.DB) *Indexer {
	return &Indexer{db: db}
}

// IndexPage upserts a single page into the computed index.
func (ix *Indexer) IndexPage(project, branch, path string, fm *schema.Frontmatter, body []byte) error {
	tags := strings.Join(fm.Tags, ",")

	_, err := ix.db.Exec(`
		INSERT INTO pages (id, title, type, status, path, project, branch, trust_level, conformance, dc_creator, dc_created, dc_modified, supersedes, superseded_by, contested_by, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title, type=excluded.type, status=excluded.status,
			path=excluded.path, branch=excluded.branch, trust_level=excluded.trust_level,
			conformance=excluded.conformance, dc_creator=excluded.dc_creator,
			dc_modified=excluded.dc_modified, supersedes=excluded.supersedes,
			superseded_by=excluded.superseded_by, contested_by=excluded.contested_by,
			tags=excluded.tags, updated_at=CURRENT_TIMESTAMP`,
		fm.ID, fm.Title, fm.Type, fm.Status, path, project, branch,
		fm.TrustLevel, fm.Conformance, fm.DCCreator,
		fm.DCCreated.Format("2006-01-02"), fm.DCModified.Format("2006-01-02"),
		fm.Supersedes, fm.SupersededBy, fm.ContestedBy, tags,
	)
	if err != nil {
		return fmt.Errorf("upsert page: %w", err)
	}

	// Index provenance edges
	_, _ = ix.db.Exec("DELETE FROM provenance_edges WHERE source_page = ?", fm.ID)
	for _, src := range fm.Provenance.Sources {
		_, err = ix.db.Exec(`
			INSERT OR REPLACE INTO provenance_edges (source_page, target_ref, target_version, target_checksum, status)
			VALUES (?, ?, ?, ?, ?)`,
			fm.ID, src.Ref, src.Version, src.Checksum, src.Status,
		)
		if err != nil {
			return fmt.Errorf("insert provenance edge: %w", err)
		}
	}

	return nil
}

// RemovePage removes a page from the index.
func (ix *Indexer) RemovePage(id string) error {
	_, err := ix.db.Exec("DELETE FROM pages WHERE id = ?", id)
	if err != nil {
		return err
	}
	_, err = ix.db.Exec("DELETE FROM provenance_edges WHERE source_page = ?", id)
	return err
}

// RebuildFromRepo rebuilds the entire index for a project/branch from git.
func (ix *Indexer) RebuildFromRepo(project string, repo *wgit.Repo, branch string) error {
	// Clear existing entries for this project
	_, _ = ix.db.Exec("DELETE FROM pages WHERE project = ?", project)
	_, _ = ix.db.Exec("DELETE FROM provenance_edges WHERE source_page IN (SELECT id FROM pages WHERE project = ?)", project)

	pages, err := repo.ListPages(branch)
	if err != nil {
		return fmt.Errorf("list pages: %w", err)
	}

	for _, path := range pages {
		if !strings.HasSuffix(path, ".md") {
			continue
		}

		fm, body, err := repo.ReadPageWithMeta(branch, path)
		if err != nil {
			continue // skip unparseable files
		}
		if fm == nil {
			continue // no frontmatter
		}

		if err := ix.IndexPage(project, branch, path, fm, body); err != nil {
			return fmt.Errorf("index %s: %w", path, err)
		}
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/index/ -v`
Expected: PASS — IndexPage and RebuildFromRepo both work

- [ ] **Step 5: Commit**

```bash
git add internal/index/indexer.go internal/index/indexer_test.go
git commit -m "feat: index builder — populate SQLite from git, upsert pages, provenance edges"
```

---

### Task 8: Search — FTS5 and Metadata Queries

**Files:**
- Create: `internal/index/search.go`
- Create: `internal/index/search_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/index/search_test.go
package index

import (
	"path/filepath"
	"testing"

	"github.com/frodex/prd2wiki/internal/schema"
)

func setupSearchDB(t *testing.T) (*Searcher, *Indexer) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	db, err := OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("OpenDatabase: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	indexer := NewIndexer(db)

	// Seed test data
	pages := []struct {
		fm   *schema.Frontmatter
		body string
	}{
		{&schema.Frontmatter{ID: "P-001", Title: "Auth Requirements", Type: "requirement", Status: "active", Tags: []string{"auth", "security"}}, "JWT tokens for authentication"},
		{&schema.Frontmatter{ID: "P-002", Title: "Session Management", Type: "concept", Status: "active", Tags: []string{"session"}}, "Session timeout and refresh logic"},
		{&schema.Frontmatter{ID: "P-003", Title: "API Design", Type: "reference", Status: "draft", Tags: []string{"api"}}, "REST API endpoint definitions"},
		{&schema.Frontmatter{ID: "P-004", Title: "Deprecated Auth", Type: "requirement", Status: "deprecated", Tags: []string{"auth"}}, "Old auth approach no longer valid"},
	}

	for _, p := range pages {
		if err := indexer.IndexPage("test-project", "truth", "pages/"+p.fm.ID+".md", p.fm, []byte(p.body)); err != nil {
			t.Fatalf("seed %s: %v", p.fm.ID, err)
		}
	}

	return NewSearcher(db), indexer
}

func TestSearchByType(t *testing.T) {
	searcher, _ := setupSearchDB(t)

	results, err := searcher.ByType("test-project", "requirement")
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}
}

func TestSearchByStatus(t *testing.T) {
	searcher, _ := setupSearchDB(t)

	results, err := searcher.ByStatus("test-project", "active")
	if err != nil {
		t.Fatalf("ByStatus: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}
}

func TestSearchByTag(t *testing.T) {
	searcher, _ := setupSearchDB(t)

	results, err := searcher.ByTag("test-project", "auth")
	if err != nil {
		t.Fatalf("ByTag: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/index/ -v -run TestSearch`
Expected: FAIL — NewSearcher doesn't exist

- [ ] **Step 3: Implement search**

```go
// internal/index/search.go
package index

import "database/sql"

type PageResult struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Path        string `json:"path"`
	Project     string `json:"project"`
	TrustLevel  int    `json:"trust_level"`
	Tags        string `json:"tags"`
}

type Searcher struct {
	db *sql.DB
}

func NewSearcher(db *sql.DB) *Searcher {
	return &Searcher{db: db}
}

func (s *Searcher) ByType(project, typ string) ([]PageResult, error) {
	return s.query("SELECT id, title, type, status, path, project, trust_level, tags FROM pages WHERE project = ? AND type = ?", project, typ)
}

func (s *Searcher) ByStatus(project, status string) ([]PageResult, error) {
	return s.query("SELECT id, title, type, status, path, project, trust_level, tags FROM pages WHERE project = ? AND status = ?", project, status)
}

func (s *Searcher) ByTag(project, tag string) ([]PageResult, error) {
	return s.query("SELECT id, title, type, status, path, project, trust_level, tags FROM pages WHERE project = ? AND tags LIKE ?", project, "%"+tag+"%")
}

func (s *Searcher) DependentsOf(ref string) ([]PageResult, error) {
	return s.query(`
		SELECT p.id, p.title, p.type, p.status, p.path, p.project, p.trust_level, p.tags
		FROM pages p
		JOIN provenance_edges e ON e.source_page = p.id
		WHERE e.target_ref = ?`, ref)
}

func (s *Searcher) query(q string, args ...interface{}) ([]PageResult, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []PageResult
	for rows.Next() {
		var r PageResult
		if err := rows.Scan(&r.ID, &r.Title, &r.Type, &r.Status, &r.Path, &r.Project, &r.TrustLevel, &r.Tags); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/index/ -v`
Expected: PASS — all search tests

- [ ] **Step 5: Commit**

```bash
git add internal/index/search.go internal/index/search_test.go
git commit -m "feat: search — by type, status, tag, and provenance dependents"
```

---

### Task 9: REST API — Server and Page CRUD

**Files:**
- Create: `internal/api/server.go`
- Create: `internal/api/pages.go`
- Create: `internal/api/pages_test.go`

- [ ] **Step 1: Write failing test for page API**

```go
// internal/api/pages_test.go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
)

func setupTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()

	repo, err := wgit.InitRepo(dir, "test-project")
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	dbPath := filepath.Join(dir, "index.db")
	db, err := index.OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("OpenDatabase: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	srv := NewServer(":0", map[string]*wgit.Repo{"test-project": repo}, db)
	return srv
}

func TestCreateAndGetPage(t *testing.T) {
	srv := setupTestServer(t)

	// Create a page
	body := map[string]interface{}{
		"id":    "PRD-001",
		"title": "Test Page",
		"type":  "concept",
		"body":  "# Hello World",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/projects/test-project/pages", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, body = %s", w.Code, w.Body.String())
	}

	// Read it back
	req = httptest.NewRequest("GET", "/api/projects/test-project/pages/PRD-001", nil)
	w = httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["id"] != "PRD-001" {
		t.Errorf("id = %v", resp["id"])
	}
	if resp["title"] != "Test Page" {
		t.Errorf("title = %v", resp["title"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -v -run TestCreateAndGet`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Implement server and page handlers**

```go
// internal/api/server.go
package api

import (
	"database/sql"
	"net/http"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
)

type Server struct {
	addr    string
	repos   map[string]*wgit.Repo
	db      *sql.DB
	indexer *index.Indexer
	search  *index.Searcher
}

func NewServer(addr string, repos map[string]*wgit.Repo, db *sql.DB) *Server {
	return &Server{
		addr:    addr,
		repos:   repos,
		db:      db,
		indexer: index.NewIndexer(db),
		search:  index.NewSearcher(db),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Page CRUD
	mux.HandleFunc("POST /api/projects/{project}/pages", s.createPage)
	mux.HandleFunc("GET /api/projects/{project}/pages/{id}", s.getPage)
	mux.HandleFunc("PUT /api/projects/{project}/pages/{id}", s.updatePage)
	mux.HandleFunc("DELETE /api/projects/{project}/pages/{id}", s.deletePage)
	mux.HandleFunc("GET /api/projects/{project}/pages", s.listPages)

	return mux
}

func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.addr, s.Handler())
}
```

```go
// internal/api/pages.go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/frodex/prd2wiki/internal/schema"
)

type CreatePageRequest struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Type   string   `json:"type"`
	Status string   `json:"status"`
	Body   string   `json:"body"`
	Tags   []string `json:"tags"`
	Branch string   `json:"branch"`
	Intent string   `json:"intent"` // verbatim | conform | integrate
	Author string   `json:"author"`
}

func (s *Server) createPage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	repo, ok := s.repos[project]
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	var req CreatePageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Status == "" {
		req.Status = "draft"
	}
	if req.Branch == "" {
		req.Branch = "draft/incoming"
	}
	if req.Author == "" {
		req.Author = "anonymous@prd2wiki"
	}

	fm := &schema.Frontmatter{
		ID:     req.ID,
		Title:  req.Title,
		Type:   req.Type,
		Status: req.Status,
		Tags:   req.Tags,
	}

	// Validate schema
	issues := schema.Validate(fm)
	if schema.HasErrors(issues) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid":  false,
			"issues": issues,
		})
		return
	}

	path := fmt.Sprintf("pages/%s.md", req.ID)
	err := repo.WritePageWithMeta(req.Branch, path, fm, []byte(req.Body), "create: "+req.Title, req.Author)
	if err != nil {
		http.Error(w, "write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update index
	_ = s.indexer.IndexPage(project, req.Branch, path, fm, []byte(req.Body))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":     fm.ID,
		"title":  fm.Title,
		"status": fm.Status,
		"path":   path,
		"issues": issues,
	})
}

func (s *Server) getPage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "truth"
	}

	repo, ok := s.repos[project]
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	path := fmt.Sprintf("pages/%s.md", id)

	// Try specified branch, fall back to draft/incoming
	fm, body, err := repo.ReadPageWithMeta(branch, path)
	if err != nil {
		http.Error(w, "page not found: "+err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":          fm.ID,
		"title":       fm.Title,
		"type":        fm.Type,
		"status":      fm.Status,
		"trust_level": fm.TrustLevel,
		"tags":        fm.Tags,
		"provenance":  fm.Provenance,
		"body":        string(body),
	})
}

func (s *Server) updatePage(w http.ResponseWriter, r *http.Request) {
	// Similar to createPage but updates existing
	s.createPage(w, r) // reuse upsert logic for now
}

func (s *Server) deletePage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "draft/incoming"
	}

	repo, ok := s.repos[project]
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	path := fmt.Sprintf("pages/%s.md", id)
	err := repo.DeletePage(branch, path, "delete: "+id, "system@prd2wiki")
	if err != nil {
		http.Error(w, "delete failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.indexer.RemovePage(id)

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listPages(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	typ := r.URL.Query().Get("type")
	status := r.URL.Query().Get("status")
	tag := r.URL.Query().Get("tag")

	var results []index.PageResult
	var err error

	switch {
	case typ != "":
		results, err = s.search.ByType(project, typ)
	case status != "":
		results, err = s.search.ByStatus(project, status)
	case tag != "":
		results, err = s.search.ByTag(project, tag)
	default:
		results, err = s.search.ByType(project, "") // all pages — needs a ListAll method
		if err != nil {
			// Fallback: query all
			results, err = s.search.ByStatus(project, "")
		}
	}

	if err != nil {
		http.Error(w, "search failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/ -v`
Expected: PASS — create and get page round-trip works

- [ ] **Step 5: Commit**

```bash
git add internal/api/
git commit -m "feat: REST API — page CRUD with schema validation and index updates"
```

---

### Task 10: Wire Everything in main.go

**Files:**
- Modify: `cmd/prd2wiki/main.go`

- [ ] **Step 1: Implement full main.go with config loading**

```go
// cmd/prd2wiki/main.go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/frodex/prd2wiki/internal/api"
	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
)

type Config struct {
	Server struct {
		Addr string `yaml:"addr"`
	} `yaml:"server"`
	Data struct {
		Dir string `yaml:"dir"`
	} `yaml:"data"`
	Projects []string `yaml:"projects"`
}

func main() {
	configPath := flag.String("config", "config/prd2wiki.yaml", "path to config file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.Data.Dir, 0755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	// Open/init git repos for each project
	repos := make(map[string]*wgit.Repo)
	for _, project := range cfg.Projects {
		repoPath := filepath.Join(cfg.Data.Dir, project+".wiki.git")
		var repo *wgit.Repo
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			repo, err = wgit.InitRepo(cfg.Data.Dir, project)
			if err != nil {
				log.Fatalf("init repo %s: %v", project, err)
			}
			log.Printf("initialized new wiki repo for project: %s", project)
		} else {
			repo, err = wgit.OpenRepo(cfg.Data.Dir, project)
			if err != nil {
				log.Fatalf("open repo %s: %v", project, err)
			}
		}
		repos[project] = repo
	}

	// Open SQLite index
	dbPath := filepath.Join(cfg.Data.Dir, "index.db")
	db, err := index.OpenDatabase(dbPath)
	if err != nil {
		log.Fatalf("open index: %v", err)
	}
	defer db.Close()

	// Rebuild index from repos
	indexer := index.NewIndexer(db)
	for project, repo := range repos {
		if err := indexer.RebuildFromRepo(project, repo, "truth"); err != nil {
			log.Printf("warning: rebuild index for %s: %v", project, err)
		}
	}

	// Start API server
	srv := api.NewServer(cfg.Server.Addr, repos, db)
	log.Printf("prd2wiki listening on %s", cfg.Server.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}
	if cfg.Data.Dir == "" {
		cfg.Data.Dir = "./data"
	}
	if len(cfg.Projects) == 0 {
		cfg.Projects = []string{"default"}
	}

	return &cfg, nil
}
```

- [ ] **Step 2: Update config with projects list**

```yaml
# config/prd2wiki.yaml
server:
  addr: ":8080"

data:
  dir: "./data"

projects:
  - default
```

- [ ] **Step 3: Build and run**

Run: `make build && ./bin/prd2wiki`
Expected: "prd2wiki listening on :8080" and server responds to requests

- [ ] **Step 4: Smoke test with curl**

```bash
# Create a page
curl -s -X POST http://localhost:8080/api/projects/default/pages \
  -H "Content-Type: application/json" \
  -d '{"id":"TEST-001","title":"Smoke Test","type":"concept","body":"# Hello prd2wiki"}' | jq .

# Read it back
curl -s http://localhost:8080/api/projects/default/pages/TEST-001?branch=draft/incoming | jq .
```

Expected: Page created and readable via API

- [ ] **Step 5: Commit**

```bash
git add cmd/prd2wiki/main.go config/prd2wiki.yaml
git commit -m "feat: wire up main — config loading, repo init, index rebuild, API server"
```

---

### Task 11: Search and References API Endpoints

**Files:**
- Create: `internal/api/search.go`
- Create: `internal/api/search_test.go`
- Create: `internal/api/references.go`
- Create: `internal/api/references_test.go`
- Modify: `internal/api/server.go` (add routes)
- Modify: `internal/index/search.go` (add ListAll, DependentsOf)

- [ ] **Step 1: Add ListAll to Searcher**

```go
// Add to internal/index/search.go

func (s *Searcher) ListAll(project string) ([]PageResult, error) {
	return s.query("SELECT id, title, type, status, path, project, trust_level, tags FROM pages WHERE project = ?", project)
}
```

- [ ] **Step 2: Add search and references routes to server.go**

```go
// Add to Handler() in internal/api/server.go

// Search
mux.HandleFunc("GET /api/projects/{project}/search", s.searchPages)

// References
mux.HandleFunc("GET /api/projects/{project}/pages/{id}/references", s.getReferences)
```

- [ ] **Step 3: Implement search handler**

```go
// internal/api/search.go
package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) searchPages(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	q := r.URL.Query().Get("q")
	typ := r.URL.Query().Get("type")
	status := r.URL.Query().Get("status")
	tag := r.URL.Query().Get("tag")

	var results []index.PageResult
	var err error

	switch {
	case q != "":
		// TODO: FTS5 full-text search — Phase 1 delivers metadata queries
		// Full-text will be added when FTS triggers are implemented
		results, err = s.search.ListAll(project)
	case typ != "":
		results, err = s.search.ByType(project, typ)
	case status != "":
		results, err = s.search.ByStatus(project, status)
	case tag != "":
		results, err = s.search.ByTag(project, tag)
	default:
		results, err = s.search.ListAll(project)
	}

	if err != nil {
		http.Error(w, "search failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
```

- [ ] **Step 4: Implement references handler**

```go
// internal/api/references.go
package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

type RefNode struct {
	Ref        string    `json:"ref"`
	Title      string    `json:"title,omitempty"`
	Version    int       `json:"version,omitempty"`
	Checksum   string    `json:"checksum,omitempty"`
	Status     string    `json:"status"`
	TrustLevel int       `json:"trust_level,omitempty"`
	Children   []RefNode `json:"children"`
}

func (s *Server) getReferences(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")
	depthStr := r.URL.Query().Get("depth")
	depth := 1
	if d, err := strconv.Atoi(depthStr); err == nil && d > 0 {
		depth = d
	}
	if depth > 5 {
		depth = 5 // cap to prevent runaway recursion
	}

	tree := s.buildRefTree(project, id, depth, 0)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"page_id": id,
		"hard":    tree,
	})
}

func (s *Server) buildRefTree(project, pageID string, maxDepth, currentDepth int) []RefNode {
	if currentDepth >= maxDepth {
		return nil
	}

	rows, err := s.db.Query(`
		SELECT e.target_ref, e.target_version, e.target_checksum, e.status, 
		       COALESCE(p.title, ''), COALESCE(p.trust_level, 0)
		FROM provenance_edges e
		LEFT JOIN pages p ON p.id = e.target_ref
		WHERE e.source_page = ?`, pageID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var nodes []RefNode
	for rows.Next() {
		var n RefNode
		var title string
		var trustLevel int
		if err := rows.Scan(&n.Ref, &n.Version, &n.Checksum, &n.Status, &title, &trustLevel); err != nil {
			continue
		}
		n.Title = title
		n.TrustLevel = trustLevel
		n.Children = s.buildRefTree(project, n.Ref, maxDepth, currentDepth+1)
		nodes = append(nodes, n)
	}
	return nodes
}
```

- [ ] **Step 5: Add missing import in references.go**

The `database/sql` import is needed but may already be available through the server struct. Verify and fix compilation.

- [ ] **Step 6: Run all tests**

Run: `make test`
Expected: PASS — all tests across all packages

- [ ] **Step 7: Commit**

```bash
git add internal/api/ internal/index/search.go
git commit -m "feat: search and reference tree API endpoints"
```

---

### Task 12: Dockerfile and Docker Compose

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yaml`

- [ ] **Step 1: Create Dockerfile**

```dockerfile
# Dockerfile
FROM golang:1.24-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o prd2wiki ./cmd/prd2wiki

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/prd2wiki /usr/local/bin/prd2wiki
COPY config/prd2wiki.yaml /etc/prd2wiki/prd2wiki.yaml

EXPOSE 8080
VOLUME /data

CMD ["prd2wiki", "-config", "/etc/prd2wiki/prd2wiki.yaml"]
```

- [ ] **Step 2: Create docker-compose.yaml**

```yaml
# docker-compose.yaml
services:
  prd2wiki:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - wiki-data:/data
      - ./config:/etc/prd2wiki
    restart: unless-stopped

volumes:
  wiki-data:
```

- [ ] **Step 3: Build and test Docker image**

```bash
docker compose build
docker compose up -d
curl -s http://localhost:8080/api/projects/default/pages | jq .
docker compose down
```

Expected: Container builds, starts, responds to API requests

- [ ] **Step 4: Commit**

```bash
git add Dockerfile docker-compose.yaml
git commit -m "feat: Docker deployment — multi-stage build, compose with volume"
```

---

## Self-Review

**Spec coverage check:**
- [x] Section 3 (Architecture): Go binary, net/http, go-git, SQLite — Task 1, 4, 6, 9, 10
- [x] Section 4 (Content Model): Frontmatter parsing, types, validation — Task 2, 3
- [x] Section 5 (Branching): Branch structure, read/write to branches — Task 4, 5
- [x] Section 9 (Access Control): Not in Phase 1 — deferred to Phase 2+ (OIDC/RBAC)
- [x] Section 10 (Librarian): Not in Phase 1 — Phase 2
- [x] Section 14 (References): Hard reference tree API — Task 11
- [x] Section 15 (Standards): Dublin Core, PROV-DM in frontmatter — Task 2

**Not in Phase 1 (by design):**
- Librarian pipeline (Phase 2)
- LanceDB vector index (Phase 2)
- Web UI (Phase 3)
- MCP sidecar (Phase 4)
- Steward agents (Phase 5)
- OIDC authentication (Phase 2+)
- Soft references (Phase 2 — requires vector DB)
- Challenge flow automation (Phase 5)
- Git commit signing (Phase 2+)

**Type consistency check:** `Frontmatter`, `PageResult`, `RefNode`, `Issue` — all consistent across packages. `Repo` methods match between git and api packages.

**Placeholder scan:** The `searchPages` handler has a TODO for FTS5 full-text search. This is acknowledged — FTS5 trigger setup for auto-indexing body content will be added as a follow-up in this phase or early Phase 2. Metadata queries (by type, status, tag) are fully functional.

---

**Phase 1 delivers:** A working wiki with git-backed storage, schema validation, SQLite index, REST API, and Docker deployment. You can create, read, update, delete pages; search by metadata; and query provenance reference trees — all via curl or any HTTP client. This is the foundation Phase 2-5 build on.

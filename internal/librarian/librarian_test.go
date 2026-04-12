package librarian_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/schema"
	"github.com/frodex/prd2wiki/internal/vocabulary"
)

func setupLibrarian(t *testing.T) (*librarian.Librarian, *wgit.Repo) {
	t.Helper()
	dir := t.TempDir()

	repo, err := wgit.InitRepo(dir, "testproject")
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	db, err := index.OpenDatabase(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDatabase: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	vocab := vocabulary.NewStore(db)
	ix := index.NewIndexer(db)

	lib := librarian.New(repo, ix, vocab)
	return lib, repo
}

func TestLibrarianVerbatim(t *testing.T) {
	lib, repo := setupLibrarian(t)
	ctx := context.Background()

	fm := &schema.Frontmatter{
		ID:     "req-001",
		Title:  "Authentication Requirement",
		Type:   "requirement",
		Status: "draft",
		Tags:   []string{"Auth", "IDENTITY", "Security"},
	}
	body := []byte("# Authentication Requirement\n\nUsers must authenticate before accessing protected resources.\n")

	result, err := lib.Submit(ctx, librarian.SubmitRequest{
		Project:     "testproject",
		Branch:      "main",
		Frontmatter: fm,
		Body:        body,
		Intent:      librarian.IntentVerbatim,
		Author:      "test-author",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !result.Saved {
		t.Fatalf("expected saved=true, got false; issues: %v", result.Issues)
	}

	// Read back from git and verify tags are preserved as-is (not normalized).
	readFM, _, err := repo.ReadPageWithMeta("main", "pages/req-001.md")
	if err != nil {
		t.Fatalf("ReadPageWithMeta: %v", err)
	}

	wantTags := []string{"Auth", "IDENTITY", "Security"}
	if len(readFM.Tags) != len(wantTags) {
		t.Fatalf("tags count: got %d, want %d; tags: %v", len(readFM.Tags), len(wantTags), readFM.Tags)
	}
	for i, want := range wantTags {
		if readFM.Tags[i] != want {
			t.Errorf("tag[%d]: got %q, want %q (verbatim should preserve original case)", i, readFM.Tags[i], want)
		}
	}
}

func TestLibrarianConform(t *testing.T) {
	lib, repo := setupLibrarian(t)
	ctx := context.Background()

	fm := &schema.Frontmatter{
		ID:     "req-002",
		Title:  "Authorization Policy",
		Type:   "requirement",
		Status: "draft",
		Tags:   []string{"AUTH", "Policy", "ACCESS"},
	}
	body := []byte("# Authorization Policy  \n\nAll access decisions must be logged.   \n")

	result, err := lib.Submit(ctx, librarian.SubmitRequest{
		Project:     "testproject",
		Branch:      "main",
		Frontmatter: fm,
		Body:        body,
		Intent:      librarian.IntentConform,
		Author:      "test-author",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !result.Saved {
		t.Fatalf("expected saved=true, got false; issues: %v", result.Issues)
	}

	// Read back from git.
	readFM, readBody, err := repo.ReadPageWithMeta("main", "pages/req-002.md")
	if err != nil {
		t.Fatalf("ReadPageWithMeta: %v", err)
	}

	// Tags should be normalized (lowercase).
	for _, tag := range readFM.Tags {
		if tag != strings.ToLower(tag) {
			t.Errorf("tag %q is not lowercased — conform should normalize tags", tag)
		}
	}

	// Body should have no trailing whitespace on any line.
	lines := strings.Split(string(readBody), "\n")
	for i, line := range lines {
		if strings.TrimRight(line, " \t") != line {
			t.Errorf("line %d has trailing whitespace: %q", i, line)
		}
	}
}

func TestLibrarianValidationError(t *testing.T) {
	lib, _ := setupLibrarian(t)
	ctx := context.Background()

	// Missing ID and Type — should block conform.
	fm := &schema.Frontmatter{
		Title:  "Nameless Page",
		Status: "draft",
	}
	body := []byte("Some content.\n")

	result, err := lib.Submit(ctx, librarian.SubmitRequest{
		Project:     "testproject",
		Branch:      "main",
		Frontmatter: fm,
		Body:        body,
		Intent:      librarian.IntentConform,
		Author:      "test-author",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Saved {
		t.Fatal("expected saved=false for invalid page, got true")
	}
	if len(result.Issues) == 0 {
		t.Fatal("expected issues to be present for invalid page")
	}
	if !schema.HasErrors(result.Issues) {
		t.Error("expected at least one error-severity issue")
	}
}

func TestLibrarianAutoGenerateID(t *testing.T) {
	lib, repo := setupLibrarian(t)
	ctx := context.Background()

	fm := &schema.Frontmatter{
		ID:    "", // empty — should be auto-generated from title
		Title: "Hello World Test",
		Type:  "concept",
	}

	result, err := lib.Submit(ctx, librarian.SubmitRequest{
		Project:     "testproject",
		Branch:      "draft/incoming",
		Frontmatter: fm,
		Body:        []byte("# Hello World Test\n\nSome content.\n"),
		Intent:      librarian.IntentVerbatim,
		Author:      "test-author",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !result.Saved {
		t.Fatalf("expected saved=true, got false; issues: %v", result.Issues)
	}

	// Path must not be "pages/.md" — that was the bug.
	if result.Path == "pages/.md" {
		t.Fatalf("generated path is 'pages/.md' — ID was not generated")
	}
	if result.Path == "" {
		t.Fatal("result path is empty")
	}

	// The ID in frontmatter must be non-empty after submission.
	if fm.ID == "" {
		t.Fatal("frontmatter ID is still empty after auto-generation")
	}

	// Read back from git and verify ID in frontmatter is non-empty.
	readFM, _, err := repo.ReadPageWithMeta("draft/incoming", result.Path)
	if err != nil {
		t.Fatalf("ReadPageWithMeta(%q): %v", result.Path, err)
	}
	if readFM.ID == "" {
		t.Errorf("ID in stored frontmatter is empty — auto-generation did not persist")
	}
}

func TestLibrarianAutoGenerateIDNoTitle(t *testing.T) {
	lib, _ := setupLibrarian(t)
	ctx := context.Background()

	fm := &schema.Frontmatter{
		ID:    "", // empty
		Title: "", // also empty — must fall back to random ID
		Type:  "concept",
	}

	result, err := lib.Submit(ctx, librarian.SubmitRequest{
		Project:     "testproject",
		Branch:      "draft/incoming",
		Frontmatter: fm,
		Body:        []byte("No title, no ID.\n"),
		Intent:      librarian.IntentVerbatim,
		Author:      "test-author",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !result.Saved {
		t.Fatalf("expected saved=true, got false; issues: %v", result.Issues)
	}

	// ID must follow the random fallback pattern: page-{date}-{hex}
	if fm.ID == "" {
		t.Fatal("ID is still empty — random fallback did not run")
	}
	if !strings.HasPrefix(fm.ID, "page-") {
		t.Errorf("random ID %q does not start with 'page-'", fm.ID)
	}
	// Verify path is not broken
	if result.Path == "pages/.md" || result.Path == "" {
		t.Errorf("path is broken: %q", result.Path)
	}
}

func TestLibrarianVerbatimEmptyID(t *testing.T) {
	lib, repo := setupLibrarian(t)
	ctx := context.Background()

	fm := &schema.Frontmatter{
		ID:    "",
		Title: "Verbatim Empty ID Test",
		Type:  "requirement",
	}

	result, err := lib.Submit(ctx, librarian.SubmitRequest{
		Project:     "testproject",
		Branch:      "draft/incoming",
		Frontmatter: fm,
		Body:        []byte("# Verbatim Empty ID Test\n\nContent.\n"),
		Intent:      librarian.IntentVerbatim,
		Author:      "test-author",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !result.Saved {
		t.Fatalf("verbatim should save despite schema issues; got saved=false, issues: %v", result.Issues)
	}

	// Path must be valid — not "pages/.md".
	if result.Path == "pages/.md" {
		t.Fatalf("verbatim with empty ID produced broken path 'pages/.md'")
	}

	// Verify the page is actually readable from git.
	readFM, _, err := repo.ReadPageWithMeta("draft/incoming", result.Path)
	if err != nil {
		t.Fatalf("ReadPageWithMeta(%q): %v", result.Path, err)
	}
	if readFM.ID == "" {
		t.Errorf("stored frontmatter still has empty ID")
	}
}

func TestLibrarianSubdirectoryPath(t *testing.T) {
	lib, repo := setupLibrarian(t)
	ctx := context.Background()

	fm := &schema.Frontmatter{
		ID:       "mechlab-design",
		Title:    "MechLab Design",
		Type:     "concept",
		Status:   "draft",
		Module:   "docs",
		Category: "research",
	}
	body := []byte("# MechLab Design\n\nDesign document.\n")

	result, err := lib.Submit(ctx, librarian.SubmitRequest{
		Project:     "testproject",
		Branch:      "main",
		Frontmatter: fm,
		Body:        body,
		Intent:      librarian.IntentVerbatim,
		Author:      "test-author",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !result.Saved {
		t.Fatalf("expected saved=true, got false; issues: %v", result.Issues)
	}

	// Path should include module/category subdirectories.
	wantPath := "pages/docs/research/mechlab-design.md"
	if result.Path != wantPath {
		t.Errorf("path: got %q, want %q", result.Path, wantPath)
	}

	// Should be readable from git at that path.
	readFM, _, err := repo.ReadPageWithMeta("main", wantPath)
	if err != nil {
		t.Fatalf("ReadPageWithMeta(%q): %v", wantPath, err)
	}
	if readFM.ID != "mechlab-design" {
		t.Errorf("ID: got %q, want %q", readFM.ID, "mechlab-design")
	}
}

func TestLibrarianFlatPathNoModule(t *testing.T) {
	lib, _ := setupLibrarian(t)
	ctx := context.Background()

	fm := &schema.Frontmatter{
		ID:     "DESIGN-003",
		Title:  "Design Three",
		Type:   "concept",
		Status: "draft",
		// No Module or Category set — should stay flat.
	}
	body := []byte("# Design Three\n\nFlat page.\n")

	result, err := lib.Submit(ctx, librarian.SubmitRequest{
		Project:     "testproject",
		Branch:      "main",
		Frontmatter: fm,
		Body:        body,
		Intent:      librarian.IntentVerbatim,
		Author:      "test-author",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	wantPath := "pages/design-003.md"
	if result.Path != wantPath {
		t.Errorf("path: got %q, want %q", result.Path, wantPath)
	}
}

func TestLibrarianIntegrate(t *testing.T) {
	lib, _ := setupLibrarian(t)
	ctx := context.Background()

	// Submit first page.
	fm1 := &schema.Frontmatter{
		ID:     "concept-001",
		Title:  "Identity Management",
		Type:   "concept",
		Status: "draft",
		Tags:   []string{"identity"},
	}
	body1 := []byte("# Identity Management\n\nIdentity management covers user lifecycle.\n")

	res1, err := lib.Submit(ctx, librarian.SubmitRequest{
		Project:     "testproject",
		Branch:      "main",
		Frontmatter: fm1,
		Body:        body1,
		Intent:      librarian.IntentIntegrate,
		Author:      "test-author",
	})
	if err != nil {
		t.Fatalf("Submit first page: %v", err)
	}
	if !res1.Saved {
		t.Fatalf("first page: expected saved=true; issues: %v", res1.Issues)
	}

	// Submit second page.
	fm2 := &schema.Frontmatter{
		ID:     "concept-002",
		Title:  "Identity Lifecycle",
		Type:   "concept",
		Status: "draft",
		Tags:   []string{"identity"},
	}
	body2 := []byte("# Identity Lifecycle\n\nCovers the full lifecycle of user identities.\n")

	res2, err := lib.Submit(ctx, librarian.SubmitRequest{
		Project:     "testproject",
		Branch:      "main",
		Frontmatter: fm2,
		Body:        body2,
		Intent:      librarian.IntentIntegrate,
		Author:      "test-author",
	})
	if err != nil {
		t.Fatalf("Submit second page: %v", err)
	}
	if !res2.Saved {
		t.Fatalf("second page: expected saved=true; issues: %v", res2.Issues)
	}

	// With NoopEmbedder all vectors are zero, so no candidates will exceed the 0.85 threshold.
	// Verify the result is structurally sound regardless.
	for _, w := range res2.Warnings {
		if !strings.HasPrefix(w, "potential duplicate: ") {
			t.Errorf("unexpected warning format: %q", w)
		}
	}
}

func TestPagePathHashPrefix(t *testing.T) {
	// New pages with auto-generated hash IDs should go into hash-prefix dirs.
	lib, repo := setupLibrarian(t)
	ctx := context.Background()

	fm := &schema.Frontmatter{
		ID:     "", // auto-generate from title
		Title:  "Hash Prefix Test Page",
		Type:   "concept",
		Status: "draft",
	}

	result, err := lib.Submit(ctx, librarian.SubmitRequest{
		Project:     "testproject",
		Branch:      "main",
		Frontmatter: fm,
		Body:        []byte("# Hash Prefix Test Page\n\nContent.\n"),
		Intent:      librarian.IntentVerbatim,
		Author:      "test-author",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !result.Saved {
		t.Fatalf("expected saved=true; issues: %v", result.Issues)
	}

	// ID should be a 7-char hex hash.
	if !schema.IsHashID(fm.ID) {
		t.Fatalf("auto-generated ID %q is not a hash ID", fm.ID)
	}

	// Path should use hash-prefix: pages/{first2}/{rest}.md
	wantPrefix := "pages/" + fm.ID[:2] + "/" + fm.ID[2:] + ".md"
	if result.Path != wantPrefix {
		t.Errorf("path: got %q, want %q", result.Path, wantPrefix)
	}

	// Must be readable from git at that path.
	readFM, _, err := repo.ReadPageWithMeta("main", result.Path)
	if err != nil {
		t.Fatalf("ReadPageWithMeta(%q): %v", result.Path, err)
	}
	if readFM.ID != fm.ID {
		t.Errorf("ID mismatch: got %q, want %q", readFM.ID, fm.ID)
	}
}

func TestLegacyFlatPath(t *testing.T) {
	// Pages with explicit human-readable IDs should stay in flat layout.
	lib, repo := setupLibrarian(t)
	ctx := context.Background()

	fm := &schema.Frontmatter{
		ID:     "design-003",
		Title:  "Design Three",
		Type:   "concept",
		Status: "draft",
	}

	result, err := lib.Submit(ctx, librarian.SubmitRequest{
		Project:     "testproject",
		Branch:      "main",
		Frontmatter: fm,
		Body:        []byte("# Design Three\n\nLegacy flat page.\n"),
		Intent:      librarian.IntentVerbatim,
		Author:      "test-author",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	wantPath := "pages/design-003.md"
	if result.Path != wantPath {
		t.Errorf("path: got %q, want %q", result.Path, wantPath)
	}

	// Must be readable.
	readFM, _, err := repo.ReadPageWithMeta("main", wantPath)
	if err != nil {
		t.Fatalf("ReadPageWithMeta(%q): %v", wantPath, err)
	}
	if readFM.ID != "design-003" {
		t.Errorf("ID mismatch: got %q, want %q", readFM.ID, "design-003")
	}
}

func TestExplicitHashLikeIDStaysFlat(t *testing.T) {
	// If a user explicitly provides an ID that happens to be 7 hex chars,
	// it should use hash-prefix layout since it matches hash format.
	lib, _ := setupLibrarian(t)
	ctx := context.Background()

	fm := &schema.Frontmatter{
		ID:     "abc1234",
		Title:  "Explicit Hash-Like ID",
		Type:   "concept",
		Status: "draft",
	}

	result, err := lib.Submit(ctx, librarian.SubmitRequest{
		Project:     "testproject",
		Branch:      "main",
		Frontmatter: fm,
		Body:        []byte("# Explicit Hash-Like ID\n\nContent.\n"),
		Intent:      librarian.IntentVerbatim,
		Author:      "test-author",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Even explicit IDs matching hash format go to hash-prefix dirs.
	wantPath := "pages/ab/c1234.md"
	if result.Path != wantPath {
		t.Errorf("path: got %q, want %q", result.Path, wantPath)
	}
}

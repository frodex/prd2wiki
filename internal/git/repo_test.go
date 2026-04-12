package git

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestInitRepo(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "myproject")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}
	if repo == nil {
		t.Fatal("InitRepo returned nil repo")
	}

	// Bare repo should exist at {dir}/myproject.wiki.git
	repoPath := filepath.Join(dir, "myproject.wiki.git")
	info, err := os.Stat(repoPath)
	if err != nil {
		t.Fatalf("expected repo dir at %s: %v", repoPath, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", repoPath)
	}

	// Should have a HEAD file (bare repo indicator)
	headPath := filepath.Join(repoPath, "HEAD")
	if _, err := os.Stat(headPath); err != nil {
		t.Fatalf("expected HEAD file at %s: %v", headPath, err)
	}
}

func TestOpenRepo(t *testing.T) {
	dir := t.TempDir()

	// Init first
	_, err := InitRepo(dir, "proj")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	// Open the same repo
	repo, err := OpenRepo(dir, "proj")
	if err != nil {
		t.Fatalf("OpenRepo failed: %v", err)
	}
	if repo == nil {
		t.Fatal("OpenRepo returned nil repo")
	}
}

func TestWriteAndReadPage(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "wiki")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	content := []byte("# Hello World\n\nThis is a test page.\n")
	_, err = repo.WritePage("main", "pages/test.md", content, "add test page", "Test User")
	if err != nil {
		t.Fatalf("WritePage failed: %v", err)
	}

	// Read it back
	got, err := repo.ReadPage("main", "pages/test.md")
	if err != nil {
		t.Fatalf("ReadPage failed: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("content mismatch:\n  got:  %q\n  want: %q", string(got), string(content))
	}
}

func TestWriteAndReadPage_Update(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "wiki")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	// Write initial content
	_, err = repo.WritePage("main", "pages/test.md", []byte("v1"), "initial", "Author")
	if err != nil {
		t.Fatalf("WritePage v1 failed: %v", err)
	}

	// Update
	_, err = repo.WritePage("main", "pages/test.md", []byte("v2"), "update", "Author")
	if err != nil {
		t.Fatalf("WritePage v2 failed: %v", err)
	}

	got, err := repo.ReadPage("main", "pages/test.md")
	if err != nil {
		t.Fatalf("ReadPage failed: %v", err)
	}
	if string(got) != "v2" {
		t.Fatalf("expected v2, got %q", string(got))
	}
}

func TestWritePreservesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "wiki")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	// Write two files
	_, err = repo.WritePage("main", "pages/a.md", []byte("aaa"), "add a", "Author")
	if err != nil {
		t.Fatalf("WritePage a failed: %v", err)
	}
	_, err = repo.WritePage("main", "pages/b.md", []byte("bbb"), "add b", "Author")
	if err != nil {
		t.Fatalf("WritePage b failed: %v", err)
	}

	// Both should be readable
	gotA, err := repo.ReadPage("main", "pages/a.md")
	if err != nil {
		t.Fatalf("ReadPage a failed: %v", err)
	}
	if string(gotA) != "aaa" {
		t.Fatalf("expected aaa, got %q", string(gotA))
	}

	gotB, err := repo.ReadPage("main", "pages/b.md")
	if err != nil {
		t.Fatalf("ReadPage b failed: %v", err)
	}
	if string(gotB) != "bbb" {
		t.Fatalf("expected bbb, got %q", string(gotB))
	}
}

func TestListBranches(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "wiki")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	// No branches initially
	branches, err := repo.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}
	if len(branches) != 0 {
		t.Fatalf("expected 0 branches, got %d: %v", len(branches), branches)
	}

	// Write to a branch, it should appear
	_, err = repo.WritePage("main", "pages/test.md", []byte("hi"), "init", "Author")
	if err != nil {
		t.Fatalf("WritePage failed: %v", err)
	}

	branches, err = repo.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}
	if len(branches) != 1 || branches[0] != "main" {
		t.Fatalf("expected [main], got %v", branches)
	}
}

func TestListPages(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "wiki")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	_, err = repo.WritePage("main", "pages/hello.md", []byte("h"), "add hello", "Author")
	if err != nil {
		t.Fatalf("WritePage hello failed: %v", err)
	}
	_, err = repo.WritePage("main", "pages/world.md", []byte("w"), "add world", "Author")
	if err != nil {
		t.Fatalf("WritePage world failed: %v", err)
	}
	_, err = repo.WritePage("main", "README.md", []byte("r"), "add readme", "Author")
	if err != nil {
		t.Fatalf("WritePage readme failed: %v", err)
	}

	pages, err := repo.ListPages("main")
	if err != nil {
		t.Fatalf("ListPages failed: %v", err)
	}

	sort.Strings(pages)
	expected := []string{"README.md", "pages/hello.md", "pages/world.md"}
	if len(pages) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, pages)
	}
	for i, p := range pages {
		if p != expected[i] {
			t.Fatalf("page[%d]: expected %q, got %q", i, expected[i], p)
		}
	}
}

func TestMultipleBranches(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "wiki")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	// Write to truth branch
	_, err = repo.WritePage("truth", "pages/main.md", []byte("truth content"), "truth commit", "Author")
	if err != nil {
		t.Fatalf("WritePage truth failed: %v", err)
	}

	// Write to draft branch
	_, err = repo.WritePage("draft/test", "pages/draft.md", []byte("draft content"), "draft commit", "Author")
	if err != nil {
		t.Fatalf("WritePage draft failed: %v", err)
	}

	// Both branches should exist
	branches, err := repo.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}
	sort.Strings(branches)
	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d: %v", len(branches), branches)
	}
	expectedBranches := []string{"draft/test", "truth"}
	for i, b := range branches {
		if b != expectedBranches[i] {
			t.Fatalf("branch[%d]: expected %q, got %q", i, expectedBranches[i], b)
		}
	}

	// Content should be independent
	truthContent, err := repo.ReadPage("truth", "pages/main.md")
	if err != nil {
		t.Fatalf("ReadPage truth failed: %v", err)
	}
	if string(truthContent) != "truth content" {
		t.Fatalf("truth content mismatch: %q", string(truthContent))
	}

	draftContent, err := repo.ReadPage("draft/test", "pages/draft.md")
	if err != nil {
		t.Fatalf("ReadPage draft failed: %v", err)
	}
	if string(draftContent) != "draft content" {
		t.Fatalf("draft content mismatch: %q", string(draftContent))
	}

	// Truth branch should NOT have draft page
	_, err = repo.ReadPage("truth", "pages/draft.md")
	if err == nil {
		t.Fatal("expected error reading draft page from truth branch")
	}
}

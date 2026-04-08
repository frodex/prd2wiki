package git

import (
	"fmt"
	"testing"
	"time"
)

func TestPageHistory(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "wiki")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	// Write three versions of the same page.
	for i, content := range []string{"v1", "v2", "v3"} {
		msg := "version " + content
		err := repo.WritePage("main", "pages/test.md", []byte(content), msg, "Author")
		if err != nil {
			t.Fatalf("WritePage %d failed: %v", i, err)
		}
		// Small sleep so commit times are ordered.
		time.Sleep(10 * time.Millisecond)
	}

	commits, err := repo.PageHistory("main", "pages/test.md", 0)
	if err != nil {
		t.Fatalf("PageHistory failed: %v", err)
	}

	if len(commits) != 3 {
		t.Fatalf("expected 3 commits, got %d", len(commits))
	}

	// Most recent first.
	if commits[0].Message != "version v3" {
		t.Errorf("expected first commit message %q, got %q", "version v3", commits[0].Message)
	}
	if commits[2].Message != "version v1" {
		t.Errorf("expected last commit message %q, got %q", "version v1", commits[2].Message)
	}

	// Each commit should have a non-empty hash.
	for i, c := range commits {
		if c.Hash == "" {
			t.Errorf("commit %d has empty hash", i)
		}
		if c.Author != "Author" {
			t.Errorf("commit %d author: got %q, want %q", i, c.Author, "Author")
		}
	}
}

func TestPageHistory_Limit(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "wiki")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		content := fmt.Sprintf("content version %d", i)
		err := repo.WritePage("main", "pages/test.md", []byte(content), "commit", "Author")
		if err != nil {
			t.Fatalf("WritePage %d failed: %v", i, err)
		}
	}

	commits, err := repo.PageHistory("main", "pages/test.md", 2)
	if err != nil {
		t.Fatalf("PageHistory failed: %v", err)
	}

	if len(commits) != 2 {
		t.Fatalf("expected 2 commits (limit), got %d", len(commits))
	}
}

func TestPageHistory_OnlyTracksTargetFile(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "wiki")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	// Write to two different files.
	if err := repo.WritePage("main", "pages/a.md", []byte("a1"), "add a", "Author"); err != nil {
		t.Fatalf("WritePage a failed: %v", err)
	}
	if err := repo.WritePage("main", "pages/b.md", []byte("b1"), "add b", "Author"); err != nil {
		t.Fatalf("WritePage b failed: %v", err)
	}
	if err := repo.WritePage("main", "pages/a.md", []byte("a2"), "update a", "Author"); err != nil {
		t.Fatalf("WritePage a2 failed: %v", err)
	}

	commits, err := repo.PageHistory("main", "pages/a.md", 0)
	if err != nil {
		t.Fatalf("PageHistory failed: %v", err)
	}

	// Should have 2 commits for a.md (add a, update a), not 3.
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits for a.md, got %d", len(commits))
	}
}

func TestReadPageAtCommit(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "wiki")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	// Write v1.
	err = repo.WritePage("main", "pages/test.md", []byte("version one"), "v1", "Author")
	if err != nil {
		t.Fatalf("WritePage v1 failed: %v", err)
	}

	// Get v1 hash.
	commits1, err := repo.PageHistory("main", "pages/test.md", 1)
	if err != nil {
		t.Fatalf("PageHistory after v1 failed: %v", err)
	}
	v1Hash := commits1[0].Hash

	// Write v2.
	err = repo.WritePage("main", "pages/test.md", []byte("version two"), "v2", "Author")
	if err != nil {
		t.Fatalf("WritePage v2 failed: %v", err)
	}

	// Current read should return v2.
	current, err := repo.ReadPage("main", "pages/test.md")
	if err != nil {
		t.Fatalf("ReadPage failed: %v", err)
	}
	if string(current) != "version two" {
		t.Fatalf("expected current to be v2, got %q", string(current))
	}

	// Read at v1 commit should return v1.
	v1Content, err := repo.ReadPageAtCommit(v1Hash, "pages/test.md")
	if err != nil {
		t.Fatalf("ReadPageAtCommit failed: %v", err)
	}
	if string(v1Content) != "version one" {
		t.Fatalf("expected v1 content %q, got %q", "version one", string(v1Content))
	}
}

func TestFindBranchForPage(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "wiki")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	// Write to draft branch only.
	err = repo.WritePage("draft/incoming", "pages/draft-only.md", []byte("draft"), "add draft", "Author")
	if err != nil {
		t.Fatalf("WritePage failed: %v", err)
	}

	branch, err := repo.FindBranchForPage("pages/draft-only.md")
	if err != nil {
		t.Fatalf("FindBranchForPage failed: %v", err)
	}
	if branch != "draft/incoming" {
		t.Errorf("expected branch %q, got %q", "draft/incoming", branch)
	}

	// Page that doesn't exist.
	_, err = repo.FindBranchForPage("pages/nonexistent.md")
	if err == nil {
		t.Fatal("expected error for nonexistent page, got nil")
	}
}

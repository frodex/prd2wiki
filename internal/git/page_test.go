package git

import (
	"testing"

	"github.com/frodex/prd2wiki/internal/schema"
)

func TestWriteAndReadPageWithMeta(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "wiki")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	fm := &schema.Frontmatter{
		ID:     "page-001",
		Title:  "My Test Page",
		Type:   "concept",
		Status: "draft",
	}
	body := []byte("# My Test Page\n\nThis is the body content.\n")

	err = repo.WritePageWithMeta("truth", "pages/test.md", fm, body, "add test page", "Test Author")
	if err != nil {
		t.Fatalf("WritePageWithMeta failed: %v", err)
	}

	gotFM, gotBody, err := repo.ReadPageWithMeta("truth", "pages/test.md")
	if err != nil {
		t.Fatalf("ReadPageWithMeta failed: %v", err)
	}

	if gotFM == nil {
		t.Fatal("expected frontmatter, got nil")
	}
	if gotFM.ID != fm.ID {
		t.Errorf("ID mismatch: got %q, want %q", gotFM.ID, fm.ID)
	}
	if gotFM.Title != fm.Title {
		t.Errorf("Title mismatch: got %q, want %q", gotFM.Title, fm.Title)
	}
	if gotFM.Type != fm.Type {
		t.Errorf("Type mismatch: got %q, want %q", gotFM.Type, fm.Type)
	}
	if gotFM.Status != fm.Status {
		t.Errorf("Status mismatch: got %q, want %q", gotFM.Status, fm.Status)
	}
	if string(gotBody) != string(body) {
		t.Errorf("body mismatch:\n  got:  %q\n  want: %q", string(gotBody), string(body))
	}
}

func TestDeletePage(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "wiki")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	fm := &schema.Frontmatter{
		ID:     "page-002",
		Title:  "Page To Delete",
		Type:   "spec",
		Status: "active",
	}
	body := []byte("This page will be deleted.\n")

	err = repo.WritePageWithMeta("truth", "pages/delete-me.md", fm, body, "add page to delete", "Author")
	if err != nil {
		t.Fatalf("WritePageWithMeta failed: %v", err)
	}

	// Verify it exists
	_, _, err = repo.ReadPageWithMeta("truth", "pages/delete-me.md")
	if err != nil {
		t.Fatalf("ReadPageWithMeta before delete failed: %v", err)
	}

	// Delete it
	err = repo.DeletePage("truth", "pages/delete-me.md", "delete page", "Author")
	if err != nil {
		t.Fatalf("DeletePage failed: %v", err)
	}

	// Reading after delete should return an error
	_, _, err = repo.ReadPageWithMeta("truth", "pages/delete-me.md")
	if err == nil {
		t.Fatal("expected error reading deleted page, got nil")
	}
}

func TestDeletePage_PreservesOtherFiles(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitRepo(dir, "wiki")
	if err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}

	// Write two pages
	fmA := &schema.Frontmatter{ID: "a", Title: "A", Type: "concept", Status: "active"}
	fmB := &schema.Frontmatter{ID: "b", Title: "B", Type: "concept", Status: "active"}

	if err := repo.WritePageWithMeta("truth", "pages/a.md", fmA, []byte("body a"), "add a", "Author"); err != nil {
		t.Fatalf("WritePageWithMeta a failed: %v", err)
	}
	if err := repo.WritePageWithMeta("truth", "pages/b.md", fmB, []byte("body b"), "add b", "Author"); err != nil {
		t.Fatalf("WritePageWithMeta b failed: %v", err)
	}

	// Delete only page a
	if err := repo.DeletePage("truth", "pages/a.md", "delete a", "Author"); err != nil {
		t.Fatalf("DeletePage a failed: %v", err)
	}

	// Page a should be gone
	_, _, err = repo.ReadPageWithMeta("truth", "pages/a.md")
	if err == nil {
		t.Fatal("expected error reading deleted page a, got nil")
	}

	// Page b should still exist
	gotFM, gotBody, err := repo.ReadPageWithMeta("truth", "pages/b.md")
	if err != nil {
		t.Fatalf("ReadPageWithMeta b after delete of a failed: %v", err)
	}
	if gotFM.ID != "b" {
		t.Errorf("expected ID b, got %q", gotFM.ID)
	}
	if string(gotBody) != "body b" {
		t.Errorf("expected body b, got %q", string(gotBody))
	}
}

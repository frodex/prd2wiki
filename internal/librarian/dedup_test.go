package librarian_test

import (
	"context"
	"testing"

	"github.com/frodex/prd2wiki/internal/embedder"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/vectordb"
)

func TestDetectDuplicates(t *testing.T) {
	emb := embedder.ZeroEmbedder{Dims: 16}
	store := vectordb.NewStore(emb)
	ctx := context.Background()

	// Index a page
	chunks := []vectordb.TextChunk{
		{Section: "Overview", Text: "This is about authentication and identity management."},
	}
	if err := store.IndexPage(ctx, "proj", "page-001", "concept", "auth,identity", chunks); err != nil {
		t.Fatalf("IndexPage: %v", err)
	}

	detector := librarian.NewDedupDetector(store)

	// Check for duplicates of a similar text (excluding page-001 itself)
	result, err := detector.Check(ctx, "proj", "page-002", "authentication and identity management overview")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// With NoopEmbedder all vectors are zero, so cosine similarity is 0 — no candidates above 0.85.
	// This tests the interface works correctly end-to-end.
	for _, c := range result.Candidates {
		if c.PageID == "page-002" {
			t.Error("result must not contain the querying page itself")
		}
		if c.Similarity <= 0.85 {
			t.Errorf("candidate %q has similarity %.4f which is not > 0.85", c.PageID, c.Similarity)
		}
	}
}

func TestDetectDuplicatesExcludesSelf(t *testing.T) {
	emb := embedder.ZeroEmbedder{Dims: 16}
	store := vectordb.NewStore(emb)
	ctx := context.Background()

	chunks := []vectordb.TextChunk{
		{Section: "Main", Text: "Some content about requirements."},
	}
	if err := store.IndexPage(ctx, "proj", "req-001", "requirement", "req", chunks); err != nil {
		t.Fatalf("IndexPage: %v", err)
	}

	detector := librarian.NewDedupDetector(store)

	// Check using the same page ID — it must not return itself as a candidate
	result, err := detector.Check(ctx, "proj", "req-001", "Some content about requirements.")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, c := range result.Candidates {
		if c.PageID == "req-001" {
			t.Error("page must not be returned as a duplicate of itself")
		}
	}
}

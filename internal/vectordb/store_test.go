package vectordb

import (
	"context"
	"testing"
)

// mockEmbedder produces deterministic vectors based on text content so that
// cosine similarity is meaningful in tests.
type mockEmbedder struct{}

// Embed generates a simple 4-dim vector from a single text by hashing characters.
func (m *mockEmbedder) Embed(_ context.Context, text, language string) ([]float32, error) {
	return textToVec(text), nil
}

// EmbedBatch generates simple 4-dim vectors per text by hashing characters.
func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string, language string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = textToVec(t)
	}
	return out, nil
}

func (m *mockEmbedder) EmbedQuery(_ context.Context, query, language string) ([]float32, error) {
	return textToVec(query), nil
}

// textToVec produces a deterministic unit-ish vector from the text.
// Texts starting with the same character will have high cosine similarity.
func textToVec(text string) []float32 {
	v := make([]float32, 4)
	for i, ch := range text {
		v[i%4] += float32(ch)
	}
	return v
}

func TestStoreAndSearch(t *testing.T) {
	store := NewStore(&mockEmbedder{})
	ctx := context.Background()

	chunks := []TextChunk{
		{Section: "intro", Text: "alpha beta gamma"},
		{Section: "body", Text: "delta epsilon zeta"},
	}

	if err := store.IndexPage(ctx, "proj1", "page-a", "doc", "", chunks); err != nil {
		t.Fatalf("IndexPage: %v", err)
	}

	results, err := store.Search(ctx, "proj1", "alpha beta", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	for _, r := range results {
		if r.PageID != "page-a" {
			t.Errorf("unexpected PageID: %s", r.PageID)
		}
	}
}

func TestFindSimilar(t *testing.T) {
	store := NewStore(&mockEmbedder{})
	ctx := context.Background()

	chunksA := []TextChunk{
		{Section: "s1", Text: "apple apricot avocado"},
	}
	chunksB := []TextChunk{
		{Section: "s1", Text: "apricot almond amaranth"},
	}

	if err := store.IndexPage(ctx, "proj1", "page-a", "doc", "", chunksA); err != nil {
		t.Fatalf("IndexPage A: %v", err)
	}
	if err := store.IndexPage(ctx, "proj1", "page-b", "doc", "", chunksB); err != nil {
		t.Fatalf("IndexPage B: %v", err)
	}

	results, err := store.FindSimilar(ctx, "proj1", "page-a", 5)
	if err != nil {
		t.Fatalf("FindSimilar: %v", err)
	}

	found := false
	for _, r := range results {
		if r.PageID == "page-a" {
			t.Error("FindSimilar should not return the source page itself")
		}
		if r.PageID == "page-b" {
			found = true
		}
	}
	if !found {
		t.Error("expected page-b in FindSimilar results")
	}
}

func TestRemovePage(t *testing.T) {
	store := NewStore(&mockEmbedder{})
	ctx := context.Background()

	chunks := []TextChunk{
		{Section: "s1", Text: "hello world foo bar"},
	}

	if err := store.IndexPage(ctx, "proj1", "page-x", "doc", "", chunks); err != nil {
		t.Fatalf("IndexPage: %v", err)
	}

	store.RemovePage("page-x")

	results, err := store.Search(ctx, "proj1", "hello", 10)
	if err != nil {
		t.Fatalf("Search after remove: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results after remove, got %d", len(results))
	}
}

func TestSearchEmptyStore(t *testing.T) {
	store := NewStore(&mockEmbedder{})
	ctx := context.Background()

	results, err := store.Search(ctx, "proj1", "anything", 10)
	if err != nil {
		t.Fatalf("Search on empty store: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty store, got %d", len(results))
	}
}

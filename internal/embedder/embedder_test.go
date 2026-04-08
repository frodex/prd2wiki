package embedder_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/frodex/prd2wiki/internal/embedder"
)

// ---------------------------------------------------------------------------
// NoopEmbedder
// ---------------------------------------------------------------------------

func TestNoopEmbedder(t *testing.T) {
	e := embedder.NewNoopEmbedder(128)

	t.Run("Available returns false", func(t *testing.T) {
		if e.Available() {
			t.Fatal("NoopEmbedder.Available() should be false")
		}
	})

	t.Run("Dimensions returns configured value", func(t *testing.T) {
		if got := e.Dimensions(); got != 128 {
			t.Fatalf("Dimensions() = %d, want 128", got)
		}
	})

	t.Run("Embed returns zero vectors of correct length", func(t *testing.T) {
		texts := []string{"hello", "world", "foo"}
		vecs, err := e.Embed(context.Background(), texts)
		if err != nil {
			t.Fatalf("Embed() error: %v", err)
		}
		if len(vecs) != len(texts) {
			t.Fatalf("got %d vectors, want %d", len(vecs), len(texts))
		}
		for i, v := range vecs {
			if len(v) != 128 {
				t.Fatalf("vector[%d] length = %d, want 128", i, len(v))
			}
			for j, f := range v {
				if f != 0 {
					t.Fatalf("vector[%d][%d] = %f, want 0", i, j, f)
				}
			}
		}
	})

	t.Run("EmbedQuery returns zero vector of correct length", func(t *testing.T) {
		vec, err := e.EmbedQuery(context.Background(), "search term")
		if err != nil {
			t.Fatalf("EmbedQuery() error: %v", err)
		}
		if len(vec) != 128 {
			t.Fatalf("EmbedQuery vector length = %d, want 128", len(vec))
		}
		for j, f := range vec {
			if f != 0 {
				t.Fatalf("EmbedQuery vector[%d] = %f, want 0", j, f)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// LlamaCppEmbedder helpers
// ---------------------------------------------------------------------------

// mockEmbeddingServer returns an httptest.Server that serves /v1/embeddings and
// /health. The handler uses fullDims for the vector length it returns; each
// float value is 0.5 for easy assertion.
func mockEmbeddingServer(t *testing.T, fullDims int) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/v1/embeddings", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		type embeddingObj struct {
			Object    string    `json:"object"`
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		type response struct {
			Object string         `json:"object"`
			Data   []embeddingObj `json:"data"`
		}

		resp := response{Object: "list"}
		for i := range req.Input {
			vec := make([]float32, fullDims)
			for j := range vec {
				vec[j] = 0.5
			}
			resp.Data = append(resp.Data, embeddingObj{
				Object:    "embedding",
				Embedding: vec,
				Index:     i,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	return httptest.NewServer(mux)
}

// ---------------------------------------------------------------------------
// LlamaCppEmbedder
// ---------------------------------------------------------------------------

func TestLlamaCppEmbedder(t *testing.T) {
	const fullDims = 768
	srv := mockEmbeddingServer(t, fullDims)
	defer srv.Close()

	e := embedder.NewLlamaCppEmbedder(srv.URL, fullDims, 5)

	t.Run("Available returns true when server healthy", func(t *testing.T) {
		if !e.Available() {
			t.Fatal("Available() should be true when server is up")
		}
	})

	t.Run("Dimensions returns configured value", func(t *testing.T) {
		if got := e.Dimensions(); got != fullDims {
			t.Fatalf("Dimensions() = %d, want %d", got, fullDims)
		}
	})

	t.Run("Embed batches texts and returns correct number of vectors", func(t *testing.T) {
		texts := []string{"alpha", "beta", "gamma"}
		vecs, err := e.Embed(context.Background(), texts)
		if err != nil {
			t.Fatalf("Embed() error: %v", err)
		}
		if len(vecs) != len(texts) {
			t.Fatalf("got %d vectors, want %d", len(vecs), len(texts))
		}
		for i, v := range vecs {
			if len(v) != fullDims {
				t.Fatalf("vector[%d] length = %d, want %d", i, len(v), fullDims)
			}
		}
	})

	t.Run("EmbedQuery returns single vector", func(t *testing.T) {
		vec, err := e.EmbedQuery(context.Background(), "find me something")
		if err != nil {
			t.Fatalf("EmbedQuery() error: %v", err)
		}
		if len(vec) != fullDims {
			t.Fatalf("EmbedQuery vector length = %d, want %d", len(vec), fullDims)
		}
	})
}

// ---------------------------------------------------------------------------
// Matryoshka truncation
// ---------------------------------------------------------------------------

func TestLlamaCppMatryoshka(t *testing.T) {
	const fullDims = 768
	const targetDims = 256

	srv := mockEmbeddingServer(t, fullDims)
	defer srv.Close()

	e := embedder.NewLlamaCppEmbedder(srv.URL, targetDims, 5)

	t.Run("Dimensions returns target dims", func(t *testing.T) {
		if got := e.Dimensions(); got != targetDims {
			t.Fatalf("Dimensions() = %d, want %d", got, targetDims)
		}
	})

	t.Run("Embed truncates full server vectors to target dims", func(t *testing.T) {
		vecs, err := e.Embed(context.Background(), []string{"truncate me"})
		if err != nil {
			t.Fatalf("Embed() error: %v", err)
		}
		if len(vecs) != 1 {
			t.Fatalf("got %d vectors, want 1", len(vecs))
		}
		if len(vecs[0]) != targetDims {
			t.Fatalf("vector length = %d, want %d (Matryoshka truncation)", len(vecs[0]), targetDims)
		}
		// Values should still be 0.5 (not corrupted by truncation)
		for j, f := range vecs[0] {
			if f != 0.5 {
				t.Fatalf("vecs[0][%d] = %f, want 0.5", j, f)
			}
		}
	})

	t.Run("EmbedQuery truncates to target dims", func(t *testing.T) {
		vec, err := e.EmbedQuery(context.Background(), "shrink this")
		if err != nil {
			t.Fatalf("EmbedQuery() error: %v", err)
		}
		if len(vec) != targetDims {
			t.Fatalf("EmbedQuery vector length = %d, want %d", len(vec), targetDims)
		}
	})
}

// ---------------------------------------------------------------------------
// Unavailable server
// ---------------------------------------------------------------------------

func TestLlamaCppUnavailable(t *testing.T) {
	// Point at a port that has nothing listening.
	e := embedder.NewLlamaCppEmbedder("http://127.0.0.1:19999", 768, 1)

	t.Run("Available returns false when server unreachable", func(t *testing.T) {
		if e.Available() {
			t.Fatal("Available() should be false when no server is running")
		}
	})
}

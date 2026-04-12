package embedder_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/frodex/prd2wiki/internal/embedder"
)

// ---------------------------------------------------------------------------
// NoopEmbedder
// ---------------------------------------------------------------------------

func TestNoopEmbedder(t *testing.T) {
	e := embedder.NoopEmbedder{}

	t.Run("Embed returns ErrEmbedderNotConfigured", func(t *testing.T) {
		_, err := e.Embed(context.Background(), "hello", "en")
		if !errors.Is(err, embedder.ErrEmbedderNotConfigured) {
			t.Fatalf("Embed() error = %v, want ErrEmbedderNotConfigured", err)
		}
	})

	t.Run("EmbedBatch returns ErrEmbedderNotConfigured", func(t *testing.T) {
		_, err := e.EmbedBatch(context.Background(), []string{"hello", "world"}, "en")
		if !errors.Is(err, embedder.ErrEmbedderNotConfigured) {
			t.Fatalf("EmbedBatch() error = %v, want ErrEmbedderNotConfigured", err)
		}
	})

	t.Run("EmbedQuery returns ErrEmbedderNotConfigured", func(t *testing.T) {
		_, err := e.EmbedQuery(context.Background(), "search term", "en")
		if !errors.Is(err, embedder.ErrEmbedderNotConfigured) {
			t.Fatalf("EmbedQuery() error = %v, want ErrEmbedderNotConfigured", err)
		}
	})

	t.Run("honors context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := e.Embed(ctx, "hello", "en")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Embed() with cancelled ctx: error = %v, want context.Canceled", err)
		}
	})
}

// ---------------------------------------------------------------------------
// OpenAIEmbedder helpers
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
			Embedding []float64 `json:"embedding"`
			Index     int       `json:"index"`
		}
		type response struct {
			Object string         `json:"object"`
			Data   []embeddingObj `json:"data"`
		}

		resp := response{Object: "list"}
		for i := range req.Input {
			vec := make([]float64, fullDims)
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
// OpenAIEmbedder
// ---------------------------------------------------------------------------

func TestOpenAIEmbedder(t *testing.T) {
	const fullDims = 768
	srv := mockEmbeddingServer(t, fullDims)
	defer srv.Close()

	cfg := embedder.EmbedderConfig{
		Endpoint:      srv.URL,
		Dimensions:    fullDims,
		TimeoutStr:    "5s",
		QueryPrefix:   "search_query: ",
		PassagePrefix: "search_document: ",
	}
	e := embedder.NewOpenAIEmbedder(cfg)

	t.Run("HealthCheck succeeds when server healthy", func(t *testing.T) {
		if err := e.HealthCheck(context.Background()); err != nil {
			t.Fatalf("HealthCheck() should succeed when server is up: %v", err)
		}
	})

	t.Run("EmbedBatch returns correct number of vectors", func(t *testing.T) {
		texts := []string{"alpha", "beta", "gamma"}
		vecs, err := e.EmbedBatch(context.Background(), texts, "en")
		if err != nil {
			t.Fatalf("EmbedBatch() error: %v", err)
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

	t.Run("Embed returns single vector", func(t *testing.T) {
		vec, err := e.Embed(context.Background(), "test text", "en")
		if err != nil {
			t.Fatalf("Embed() error: %v", err)
		}
		if len(vec) != fullDims {
			t.Fatalf("Embed vector length = %d, want %d", len(vec), fullDims)
		}
	})

	t.Run("EmbedQuery returns single vector", func(t *testing.T) {
		vec, err := e.EmbedQuery(context.Background(), "find me something", "en")
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

	cfg := embedder.EmbedderConfig{
		Endpoint:      srv.URL,
		Dimensions:    targetDims,
		TimeoutStr:    "5s",
		QueryPrefix:   "search_query: ",
		PassagePrefix: "search_document: ",
	}
	e := embedder.NewOpenAIEmbedder(cfg)

	t.Run("EmbedBatch truncates full server vectors to target dims", func(t *testing.T) {
		vecs, err := e.EmbedBatch(context.Background(), []string{"truncate me"}, "en")
		if err != nil {
			t.Fatalf("EmbedBatch() error: %v", err)
		}
		if len(vecs) != 1 {
			t.Fatalf("got %d vectors, want 1", len(vecs))
		}
		if len(vecs[0]) != targetDims {
			t.Fatalf("vector length = %d, want %d (Matryoshka truncation)", len(vecs[0]), targetDims)
		}
	})

	t.Run("EmbedQuery truncates to target dims", func(t *testing.T) {
		vec, err := e.EmbedQuery(context.Background(), "shrink this", "en")
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
	cfg := embedder.EmbedderConfig{
		Endpoint:      "http://127.0.0.1:19999",
		Dimensions:    768,
		TimeoutStr:    "1s",
		QueryPrefix:   "search_query: ",
		PassagePrefix: "search_document: ",
	}
	e := embedder.NewOpenAIEmbedder(cfg)

	t.Run("HealthCheck returns error when server unreachable", func(t *testing.T) {
		if err := e.HealthCheck(context.Background()); err == nil {
			t.Fatal("HealthCheck() should return error when no server is running")
		}
	})
}

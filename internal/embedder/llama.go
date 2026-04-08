package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	queryPrefix    = "search_query: "
	documentPrefix = "search_document: "
)

// LlamaCppEmbedder calls a llama.cpp server's /v1/embeddings endpoint.
type LlamaCppEmbedder struct {
	endpoint string
	dims     int
	client   *http.Client
}

// NewLlamaCppEmbedder creates an embedder that talks to the llama.cpp server at
// endpoint, truncates vectors to dims (Matryoshka), and uses timeoutSec per
// request.
func NewLlamaCppEmbedder(endpoint string, dims int, timeoutSec int) *LlamaCppEmbedder {
	return &LlamaCppEmbedder{
		endpoint: endpoint,
		dims:     dims,
		client:   &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

// Available probes /health with a 2-second timeout.
func (e *LlamaCppEmbedder) Available() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.endpoint+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Dimensions returns the configured (possibly truncated) vector length.
func (e *LlamaCppEmbedder) Dimensions() int { return e.dims }

// Embed encodes a batch of passage texts. Each text is prefixed with
// "search_document: " before being sent to the model.
func (e *LlamaCppEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	prefixed := make([]string, len(texts))
	for i, t := range texts {
		prefixed[i] = documentPrefix + t
	}
	return e.callEmbeddings(ctx, prefixed)
}

// EmbedQuery encodes a single query string, prefixed with "search_query: ".
func (e *LlamaCppEmbedder) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	vecs, err := e.callEmbeddings(ctx, []string{queryPrefix + query})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("llama.cpp returned no embeddings")
	}
	return vecs[0], nil
}

// ---------------------------------------------------------------------------
// internal
// ---------------------------------------------------------------------------

type embeddingRequest struct {
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (e *LlamaCppEmbedder) callEmbeddings(ctx context.Context, inputs []string) ([][]float32, error) {
	body, err := json.Marshal(embeddingRequest{Input: inputs})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST /v1/embeddings: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llama.cpp returned HTTP %d", resp.StatusCode)
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Re-order by index so callers always receive vectors in input order.
	ordered := make([][]float32, len(inputs))
	for _, d := range result.Data {
		if d.Index < 0 || d.Index >= len(inputs) {
			return nil, fmt.Errorf("unexpected embedding index %d", d.Index)
		}
		ordered[d.Index] = e.truncate(d.Embedding)
	}
	return ordered, nil
}

// truncate applies Matryoshka truncation: if the server returned more
// dimensions than configured, keep only the first e.dims values.
func (e *LlamaCppEmbedder) truncate(v []float32) []float32 {
	if len(v) <= e.dims {
		return v
	}
	return v[:e.dims]
}

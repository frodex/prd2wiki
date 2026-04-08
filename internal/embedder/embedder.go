package embedder

import "context"

// Embedder converts text into dense float vectors for semantic search.
type Embedder interface {
	// Embed encodes a batch of passage texts into vectors.
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// EmbedQuery encodes a single search query into a vector.
	EmbedQuery(ctx context.Context, query string) ([]float32, error)

	// Dimensions returns the length of vectors produced by this embedder.
	Dimensions() int

	// Available reports whether the embedding backend is reachable and ready.
	Available() bool
}

// ---------------------------------------------------------------------------
// NoopEmbedder
// ---------------------------------------------------------------------------

// NoopEmbedder is a zero-value fallback that returns zero vectors.
// Available() always returns false so callers can degrade gracefully.
type NoopEmbedder struct {
	dims int
}

// NewNoopEmbedder creates a NoopEmbedder that produces zero vectors of length dims.
func NewNoopEmbedder(dims int) *NoopEmbedder {
	return &NoopEmbedder{dims: dims}
}

// Available always returns false for the noop implementation.
func (n *NoopEmbedder) Available() bool { return false }

// Dimensions returns the configured vector length.
func (n *NoopEmbedder) Dimensions() int { return n.dims }

// Embed returns one zero vector per input text.
func (n *NoopEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, n.dims)
	}
	return out, nil
}

// EmbedQuery returns a single zero vector.
func (n *NoopEmbedder) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, n.dims), nil
}

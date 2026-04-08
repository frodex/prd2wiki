package embedder

import (
	"context"
	"errors"
)

// DefaultLanguage is the default language code for embedding operations.
const DefaultLanguage = "en"

var ErrEmbedderNotConfigured = errors.New("embedder: embedder not configured")

// Embedder converts text into vectors. Implementations can be local or remote,
// but must honor context cancellation.
//
// Embed/EmbedBatch encode passage (document) text. EmbedQuery encodes query
// text — instruction-tuned models use different prefixes for each direction.
type Embedder interface {
	Embed(ctx context.Context, text, language string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string, language string) ([][]float32, error)
	EmbedQuery(ctx context.Context, query, language string) ([]float32, error)
}

// EmbedBatchHelper is a convenience helper that always uses context-aware embedding.
func EmbedBatchHelper(ctx context.Context, e Embedder, texts []string, language string) ([][]float32, error) {
	if e == nil {
		return nil, ErrEmbedderNotConfigured
	}
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	return e.EmbedBatch(ctx, texts, language)
}

// NoopEmbedder is a safe default for bootstrapping when no provider exists yet.
type NoopEmbedder struct{}

func (NoopEmbedder) Embed(ctx context.Context, text, language string) ([]float32, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return nil, ErrEmbedderNotConfigured
}

func (NoopEmbedder) EmbedBatch(ctx context.Context, texts []string, language string) ([][]float32, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return nil, ErrEmbedderNotConfigured
}

func (n NoopEmbedder) EmbedQuery(ctx context.Context, query, language string) ([]float32, error) {
	return n.Embed(ctx, query, language)
}

// ZeroEmbedder returns zero vectors of a fixed dimension. Useful for tests
// where the embedder must succeed but the actual vectors don't matter.
type ZeroEmbedder struct {
	Dims int
}

func (z ZeroEmbedder) Embed(_ context.Context, text, language string) ([]float32, error) {
	return make([]float32, z.Dims), nil
}

func (z ZeroEmbedder) EmbedBatch(_ context.Context, texts []string, language string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, z.Dims)
	}
	return out, nil
}

func (z ZeroEmbedder) EmbedQuery(_ context.Context, query, language string) ([]float32, error) {
	return make([]float32, z.Dims), nil
}

package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
)

// EmbedderConfig holds configuration for the OpenAI-compatible embedding client.
type EmbedderConfig struct {
	Type          string `yaml:"type"`             // "openai", "openai", "noop"
	Endpoint      string `yaml:"endpoint"`         // e.g. "http://127.0.0.1:8081"
	Model         string `yaml:"model"`            // model name for API requests
	Dimensions    int    `yaml:"dimensions"`       // target dims (Matryoshka truncation if < model native)
	BatchSize     int    `yaml:"batch_size"`       // max texts per HTTP call
	TimeoutStr    string `yaml:"timeout"`          // Go duration string, e.g. "10s"
	MonitorStr    string `yaml:"monitor_interval"` // health monitor interval, e.g. "2m"
	QueryPrefix   string `yaml:"query_prefix"`     // "search_query: "
	PassagePrefix string `yaml:"passage_prefix"`   // "search_document: "
}

// ParsedTimeout returns the parsed timeout duration, defaulting to 10s.
func (c EmbedderConfig) ParsedTimeout() time.Duration {
	if c.TimeoutStr == "" {
		return 10 * time.Second
	}
	d, err := time.ParseDuration(c.TimeoutStr)
	if err != nil {
		return 10 * time.Second
	}
	return d
}

// ParsedMonitorInterval returns the parsed monitor interval, defaulting to 2m.
func (c EmbedderConfig) ParsedMonitorInterval() time.Duration {
	if c.MonitorStr == "" {
		return 2 * time.Minute
	}
	d, err := time.ParseDuration(c.MonitorStr)
	if err != nil {
		return 2 * time.Minute
	}
	return d
}

// OpenAIEmbedder calls a OpenAI-compatible /v1/embeddings endpoint.
type OpenAIEmbedder struct {
	cfg    EmbedderConfig
	client *http.Client
}

var _ Embedder = (*OpenAIEmbedder)(nil)

func NewOpenAIEmbedder(cfg EmbedderConfig) *OpenAIEmbedder {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 64
	}
	timeout := cfg.ParsedTimeout()
	return &OpenAIEmbedder{
		cfg: cfg,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, text, language string) ([]float32, error) {
	vecs, err := e.embed(ctx, []string{e.cfg.PassagePrefix + text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

func (e *OpenAIEmbedder) EmbedQuery(ctx context.Context, query, language string) ([]float32, error) {
	vecs, err := e.embed(ctx, []string{e.cfg.QueryPrefix + query})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

func (e *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string, language string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	prefixed := make([]string, len(texts))
	for i, t := range texts {
		prefixed[i] = e.cfg.PassagePrefix + t
	}

	if len(prefixed) <= e.cfg.BatchSize {
		return e.embed(ctx, prefixed)
	}

	results := make([][]float32, len(prefixed))
	g, ctx := errgroup.WithContext(ctx)

	for start := 0; start < len(prefixed); start += e.cfg.BatchSize {
		start := start
		end := start + e.cfg.BatchSize
		if end > len(prefixed) {
			end = len(prefixed)
		}
		chunk := prefixed[start:end]

		g.Go(func() error {
			vecs, err := e.embed(ctx, chunk)
			if err != nil {
				return err
			}
			copy(results[start:], vecs)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

// HealthCheck verifies the embedding server is reachable and returns vectors.
func (e *OpenAIEmbedder) HealthCheck(ctx context.Context) error {
	vec, err := e.Embed(ctx, "health check", DefaultLanguage)
	if err != nil {
		return fmt.Errorf("embedder health check: %w", err)
	}
	if len(vec) == 0 {
		return fmt.Errorf("embedder health check: empty vector")
	}
	return nil
}

type embeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model,omitempty"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (e *OpenAIEmbedder) embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(embeddingRequest{
		Input: texts,
		Model: e.cfg.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.Endpoint+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding server returned %d", resp.StatusCode)
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}

	if len(result.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(result.Data))
	}

	vecs := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("embedding response index out of range: %d", d.Index)
		}
		vec := toFloat32(d.Embedding)
		if e.cfg.Dimensions > 0 && len(vec) > e.cfg.Dimensions {
			vec = matryoshkaTruncate(vec, e.cfg.Dimensions)
		}
		vecs[d.Index] = vec
	}
	for i := range vecs {
		if len(vecs[i]) == 0 {
			return nil, fmt.Errorf("embedding response missing vector for index %d", i)
		}
	}
	return vecs, nil
}

func toFloat32(f64 []float64) []float32 {
	out := make([]float32, len(f64))
	for i, v := range f64 {
		out[i] = float32(v)
	}
	return out
}

// matryoshkaTruncate truncates a vector to dims dimensions and L2-normalizes it.
func matryoshkaTruncate(vec []float32, dims int) []float32 {
	if len(vec) <= dims {
		return vec
	}
	trunc := vec[:dims]
	var norm float64
	for _, v := range trunc {
		norm += float64(v) * float64(v)
	}
	if norm > 0 {
		scale := float32(1.0 / math.Sqrt(norm))
		for i := range trunc {
			trunc[i] *= scale
		}
	}
	return trunc
}

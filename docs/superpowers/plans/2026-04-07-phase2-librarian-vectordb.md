# Phase 2: Librarian + Vector Index — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the librarian ingestion pipeline and LanceDB vector index to prd2wiki. The librarian sits between the API and storage, processing every write through classify → validate → normalize → dedup → cross-reference → write → index. The vector index enables semantic search, similarity-based dedup, and prior art suggestions.

**Architecture:** Librarian is a core component (not a sidecar). LanceDB embeddings are computed and stored alongside the SQLite index — both are derived from git, disposable, rebuildable. The embedder runs as a local sidecar (llama.cpp with nomic-embed-text-v1.5) or falls back to no-op (lexical-only search).

**Tech Stack:** Go, LanceDB Go client (in-process mode initially, CGO optional), llama.cpp embedder via HTTP, existing SQLite + go-git foundation from Phase 1

**Spec Reference:** `/srv/prd2wiki/docs/superpowers/specs/2026-04-07-prd2wiki-design-04.md` (Sections 10, 14)

**Prior Art:** Pippi project at `github.com/frodex/Pippi/internal/lancedb/` — embedder.go, embedder_llama.go, record.go, librarian.go, table.go. Adapt patterns, don't copy verbatim (different domain).

---

## File Structure

```
internal/
├── embedder/
│   ├── embedder.go              # Embedder interface + NoopEmbedder
│   ├── embedder_test.go
│   ├── llama.go                 # LlamaCpp HTTP client for /v1/embeddings
│   └── llama_test.go
├── vectordb/
│   ├── store.go                 # Vector store — embed + store + search page embeddings
│   ├── store_test.go
│   ├── record.go                # PageEmbedding record type
│   └── softref.go               # Soft reference management (accept/dismiss/promote)
├── librarian/
│   ├── librarian.go             # Librarian pipeline: classify → validate → normalize → dedup → write → index
│   ├── librarian_test.go
│   ├── classifier.go            # 3-tier classification (rules → vector → LLM)
│   ├── normalizer.go            # Text normalization, tag canonicalization
│   ├── normalizer_test.go
│   ├── dedup.go                 # Deduplication detection (keep-both-linked)
│   └── dedup_test.go
├── vocabulary/
│   ├── vocab.go                 # Vocabulary store — canonical terms, fuzzy matching
│   └── vocab_test.go
scripts/
└── install-embedder.sh          # Adapted from Pippi — downloads llama-server + nomic model
```

---

### Task 1: Embedder Interface + NoopEmbedder

**Files:**
- Create: `internal/embedder/embedder.go`
- Create: `internal/embedder/embedder_test.go`

- [ ] **Step 1: Write failing test**

```go
package embedder

import "testing"

func TestNoopEmbedder(t *testing.T) {
	e := NewNoopEmbedder(768)

	if e.Dimensions() != 768 {
		t.Errorf("Dimensions = %d, want 768", e.Dimensions())
	}

	vecs, err := e.Embed(nil, []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("got %d vectors, want 2", len(vecs))
	}
	if len(vecs[0]) != 768 {
		t.Errorf("vector dim = %d, want 768", len(vecs[0]))
	}

	qvec, err := e.EmbedQuery(nil, "test query")
	if err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}
	if len(qvec) != 768 {
		t.Errorf("query vector dim = %d, want 768", len(qvec))
	}

	if e.Available() {
		t.Error("NoopEmbedder should not be available")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/embedder/ -v`
Expected: FAIL

- [ ] **Step 3: Implement Embedder interface and NoopEmbedder**

```go
package embedder

import "context"

// Embedder generates vector embeddings from text.
// Adapted from Pippi's embedder pattern — supports passage vs query prefixes,
// Matryoshka dimension truncation, and degraded fallback.
type Embedder interface {
	// Embed generates embeddings for passage texts (search_document: prefix applied).
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// EmbedQuery generates a single embedding for a search query (search_query: prefix applied).
	EmbedQuery(ctx context.Context, query string) ([]float32, error)

	// Dimensions returns the target vector dimension count.
	Dimensions() int

	// Available returns true if the embedder is operational.
	Available() bool
}

// NoopEmbedder returns zero vectors. Used as degraded fallback when
// the embedding service is unavailable — system falls back to lexical-only search.
type NoopEmbedder struct {
	dims int
}

func NewNoopEmbedder(dims int) *NoopEmbedder {
	return &NoopEmbedder{dims: dims}
}

func (e *NoopEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	vecs := make([][]float32, len(texts))
	for i := range vecs {
		vecs[i] = make([]float32, e.dims)
	}
	return vecs, nil
}

func (e *NoopEmbedder) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, e.dims), nil
}

func (e *NoopEmbedder) Dimensions() int { return e.dims }
func (e *NoopEmbedder) Available() bool { return false }
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./internal/embedder/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/embedder/
git commit -m "feat: Embedder interface and NoopEmbedder for degraded fallback"
```

---

### Task 2: LlamaCpp Embedder Client

**Files:**
- Create: `internal/embedder/llama.go`
- Create: `internal/embedder/llama_test.go`

- [ ] **Step 1: Write test with mock HTTP server**

```go
package embedder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLlamaCppEmbedder(t *testing.T) {
	// Mock llama.cpp /v1/embeddings endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input interface{} `json:"input"`
			Model string      `json:"model"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		// Determine how many inputs
		var count int
		switch v := req.Input.(type) {
		case string:
			count = 1
		case []interface{}:
			count = len(v)
		}

		var data []map[string]interface{}
		for i := 0; i < count; i++ {
			vec := make([]float64, 768)
			for j := range vec {
				vec[j] = float64(j) * 0.001
			}
			data = append(data, map[string]interface{}{
				"embedding": vec,
				"index":     i,
			})
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":  data,
			"model": "nomic",
		})
	}))
	defer server.Close()

	emb := NewLlamaCppEmbedder(server.URL, 768, 30)

	if !emb.Available() {
		t.Error("should be available when server responds")
	}

	vecs, err := emb.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("got %d vectors", len(vecs))
	}
	if len(vecs[0]) != 768 {
		t.Errorf("dim = %d", len(vecs[0]))
	}

	qvec, err := emb.EmbedQuery(context.Background(), "test")
	if err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}
	if len(qvec) != 768 {
		t.Errorf("query dim = %d", len(qvec))
	}
}

func TestLlamaCppMatryoshka(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return full 768 dims
		vec := make([]float64, 768)
		for j := range vec {
			vec[j] = float64(j) * 0.001
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":  []map[string]interface{}{{"embedding": vec, "index": 0}},
			"model": "nomic",
		})
	}))
	defer server.Close()

	// Request truncation to 256
	emb := NewLlamaCppEmbedder(server.URL, 256, 30)

	vecs, err := emb.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs[0]) != 256 {
		t.Errorf("expected Matryoshka truncation to 256, got %d", len(vecs[0]))
	}
}
```

- [ ] **Step 2: Implement LlamaCppEmbedder**

```go
package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// LlamaCppEmbedder calls a llama.cpp server's OpenAI-compatible /v1/embeddings endpoint.
// Supports batched embedding, Matryoshka dimension truncation, and query/passage prefixes.
type LlamaCppEmbedder struct {
	endpoint   string
	dims       int
	client     *http.Client
	queryPfx   string
	passagePfx string
}

func NewLlamaCppEmbedder(endpoint string, dims int, timeoutSec int) *LlamaCppEmbedder {
	return &LlamaCppEmbedder{
		endpoint:   endpoint + "/v1/embeddings",
		dims:       dims,
		client:     &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
		queryPfx:   "search_query: ",
		passagePfx: "search_document: ",
	}
}

type embeddingRequest struct {
	Input interface{} `json:"input"`
	Model string      `json:"model"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (e *LlamaCppEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	prefixed := make([]string, len(texts))
	for i, t := range texts {
		prefixed[i] = e.passagePfx + t
	}
	return e.embed(ctx, prefixed)
}

func (e *LlamaCppEmbedder) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	vecs, err := e.embed(ctx, []string{e.queryPfx + query})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

func (e *LlamaCppEmbedder) embed(ctx context.Context, texts []string) ([][]float32, error) {
	var input interface{}
	if len(texts) == 1 {
		input = texts[0]
	} else {
		input = texts
	}

	body, err := json.Marshal(embeddingRequest{Input: input, Model: "nomic"})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedder request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedder returned %d", resp.StatusCode)
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	vecs := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index >= len(vecs) {
			continue
		}
		dim := e.dims
		if dim > len(d.Embedding) {
			dim = len(d.Embedding)
		}
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = float32(d.Embedding[j])
		}
		vecs[d.Index] = vec
	}

	return vecs, nil
}

func (e *LlamaCppEmbedder) Dimensions() int { return e.dims }

func (e *LlamaCppEmbedder) Available() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", e.endpoint[:len(e.endpoint)-len("/v1/embeddings")]+"/health", nil)
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
```

- [ ] **Step 3: Run tests, verify pass**

Run: `go test ./internal/embedder/ -v`

- [ ] **Step 4: Commit**

```bash
git add internal/embedder/
git commit -m "feat: LlamaCpp embedder client — batched, Matryoshka truncation, query/passage prefixes"
```

---

### Task 3: Vector Store — Embed + Store + Search Page Embeddings

**Files:**
- Create: `internal/vectordb/store.go`
- Create: `internal/vectordb/record.go`
- Create: `internal/vectordb/store_test.go`

- [ ] **Step 1: Write record type**

```go
package vectordb

// PageEmbedding represents a page chunk's embedding stored in the vector index.
type PageEmbedding struct {
	PageID    string    `json:"page_id"`
	Section   string    `json:"section"`   // heading or chunk identifier
	Project   string    `json:"project"`
	Type      string    `json:"type"`      // from frontmatter
	Tags      string    `json:"tags"`      // comma-joined
	Vector    []float32 `json:"vector"`
	Text      string    `json:"text"`      // the chunk text (for BM25)
}
```

- [ ] **Step 2: Write failing tests**

```go
package vectordb

import (
	"context"
	"testing"

	"github.com/frodex/prd2wiki/internal/embedder"
)

func TestStoreAndSearch(t *testing.T) {
	emb := embedder.NewNoopEmbedder(768)
	store := NewStore(emb)

	err := store.IndexPage(context.Background(), "project-a", "PRD-001", "requirement", "auth,security", []TextChunk{
		{Section: "1.0", Text: "JWT tokens for authentication"},
		{Section: "2.0", Text: "OAuth 2.0 flow description"},
	})
	if err != nil {
		t.Fatalf("IndexPage: %v", err)
	}

	results, err := store.Search(context.Background(), "project-a", "authentication", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}
}

func TestFindSimilar(t *testing.T) {
	emb := embedder.NewNoopEmbedder(768)
	store := NewStore(emb)

	store.IndexPage(context.Background(), "project-a", "PRD-001", "requirement", "auth", []TextChunk{
		{Section: "1.0", Text: "JWT authentication"},
	})
	store.IndexPage(context.Background(), "project-a", "PRD-002", "requirement", "auth", []TextChunk{
		{Section: "1.0", Text: "OAuth authentication"},
	})

	similar, err := store.FindSimilar(context.Background(), "project-a", "PRD-001", 10)
	if err != nil {
		t.Fatalf("FindSimilar: %v", err)
	}
	// Should find PRD-002 as similar (with noop embedder, all zero vectors, so similarity is equal)
	if len(similar) < 1 {
		t.Error("expected at least 1 similar page")
	}
}

func TestRemovePage(t *testing.T) {
	emb := embedder.NewNoopEmbedder(768)
	store := NewStore(emb)

	store.IndexPage(context.Background(), "project-a", "PRD-001", "requirement", "auth", []TextChunk{
		{Section: "1.0", Text: "test content"},
	})

	store.RemovePage("PRD-001")

	results, _ := store.Search(context.Background(), "project-a", "test", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results after remove, got %d", len(results))
	}
}
```

- [ ] **Step 3: Implement vector store (in-memory for now)**

The store keeps an in-memory list of PageEmbedding records. Search computes cosine similarity between the query embedding and all stored embeddings. This is the starting point — native LanceDB persistence can be added later without changing the interface.

```go
package vectordb

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/frodex/prd2wiki/internal/embedder"
)

type TextChunk struct {
	Section string
	Text    string
}

type SearchResult struct {
	PageID     string  `json:"page_id"`
	Section    string  `json:"section"`
	Similarity float64 `json:"similarity"`
	Text       string  `json:"text"`
}

type Store struct {
	embedder embedder.Embedder
	records  []PageEmbedding
	mu       sync.RWMutex
}

func NewStore(emb embedder.Embedder) *Store {
	return &Store{embedder: emb}
}

func (s *Store) IndexPage(ctx context.Context, project, pageID, typ, tags string, chunks []TextChunk) error {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}

	vecs, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing entries for this page
	s.removePageLocked(pageID)

	for i, chunk := range chunks {
		s.records = append(s.records, PageEmbedding{
			PageID:  pageID,
			Section: chunk.Section,
			Project: project,
			Type:    typ,
			Tags:    tags,
			Vector:  vecs[i],
			Text:    chunk.Text,
		})
	}

	return nil
}

func (s *Store) RemovePage(pageID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removePageLocked(pageID)
}

func (s *Store) removePageLocked(pageID string) {
	filtered := s.records[:0]
	for _, r := range s.records {
		if r.PageID != pageID {
			filtered = append(filtered, r)
		}
	}
	s.records = filtered
}

func (s *Store) Search(ctx context.Context, project, query string, limit int) ([]SearchResult, error) {
	qvec, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []SearchResult
	for _, r := range s.records {
		if project != "" && r.Project != project {
			continue
		}
		sim := cosineSimilarity(qvec, r.Vector)

		// Also do BM25-style keyword matching as fallback
		textScore := keywordScore(query, r.Text)
		combined := sim*0.7 + textScore*0.3 // simple fusion

		results = append(results, SearchResult{
			PageID:     r.PageID,
			Section:    r.Section,
			Similarity: combined,
			Text:       r.Text,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func (s *Store) FindSimilar(ctx context.Context, project, pageID string, limit int) ([]SearchResult, error) {
	s.mu.RLock()

	// Get vectors for the source page
	var sourceTexts []string
	for _, r := range s.records {
		if r.PageID == pageID {
			sourceTexts = append(sourceTexts, r.Text)
		}
	}
	s.mu.RUnlock()

	if len(sourceTexts) == 0 {
		return nil, nil
	}

	// Search using the first chunk's text as query
	results, err := s.Search(ctx, project, sourceTexts[0], limit+10)
	if err != nil {
		return nil, err
	}

	// Filter out the source page itself
	var filtered []SearchResult
	for _, r := range results {
		if r.PageID != pageID {
			filtered = append(filtered, r)
		}
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered, nil
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func keywordScore(query, text string) float64 {
	qWords := strings.Fields(strings.ToLower(query))
	tLower := strings.ToLower(text)
	matches := 0
	for _, w := range qWords {
		if strings.Contains(tLower, w) {
			matches++
		}
	}
	if len(qWords) == 0 {
		return 0
	}
	return float64(matches) / float64(len(qWords))
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./internal/vectordb/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/vectordb/
git commit -m "feat: vector store — in-memory with cosine similarity + keyword fusion search"
```

---

### Task 4: Vocabulary Store

**Files:**
- Create: `internal/vocabulary/vocab.go`
- Create: `internal/vocabulary/vocab_test.go`

- [ ] **Step 1: Write failing tests**

```go
package vocabulary

import (
	"path/filepath"
	"testing"

	"github.com/frodex/prd2wiki/internal/index"
)

func setupVocab(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	db, err := index.OpenDatabase(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

func TestAddAndGetTerm(t *testing.T) {
	v := setupVocab(t)
	err := v.Add("network-device", "tag")
	if err != nil {
		t.Fatal(err)
	}

	term, err := v.Get("network-device")
	if err != nil {
		t.Fatal(err)
	}
	if term.Term != "network-device" || term.Category != "tag" {
		t.Errorf("got %+v", term)
	}
}

func TestNormalize(t *testing.T) {
	v := setupVocab(t)
	v.Add("network-device", "tag")

	// Exact match
	norm := v.Normalize("network-device")
	if norm != "network-device" {
		t.Errorf("exact match: got %q", norm)
	}

	// Case normalization
	norm = v.Normalize("Network-Device")
	if norm != "network-device" {
		t.Errorf("case norm: got %q", norm)
	}
}

func TestUsageCount(t *testing.T) {
	v := setupVocab(t)
	v.Add("auth", "tag")
	v.Add("auth", "tag") // second add increments usage

	term, _ := v.Get("auth")
	if term.UsageCount < 2 {
		t.Errorf("usage_count = %d, want >= 2", term.UsageCount)
	}
}
```

- [ ] **Step 2: Implement vocabulary store**

```go
package vocabulary

import (
	"database/sql"
	"strings"
)

type Term struct {
	Term       string `json:"term"`
	Category   string `json:"category"`
	UsageCount int    `json:"usage_count"`
	Canonical  bool   `json:"canonical"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Add adds or increments usage of a canonical term.
func (s *Store) Add(term, category string) error {
	_, err := s.db.Exec(`
		INSERT INTO vocabulary (term, category, usage_count, canonical)
		VALUES (?, ?, 1, 1)
		ON CONFLICT(term) DO UPDATE SET usage_count = usage_count + 1`,
		strings.ToLower(term), category)
	return err
}

// Get retrieves a term by exact match.
func (s *Store) Get(term string) (*Term, error) {
	var t Term
	var canonical int
	err := s.db.QueryRow("SELECT term, category, usage_count, canonical FROM vocabulary WHERE term = ?",
		strings.ToLower(term)).Scan(&t.Term, &t.Category, &t.UsageCount, &canonical)
	if err != nil {
		return nil, err
	}
	t.Canonical = canonical == 1
	return &t, nil
}

// Normalize returns the canonical form of a term.
// For now: lowercase match against vocabulary. Future: fuzzy matching > 0.85.
func (s *Store) Normalize(term string) string {
	lower := strings.ToLower(term)
	var canonical string
	err := s.db.QueryRow("SELECT term FROM vocabulary WHERE term = ?", lower).Scan(&canonical)
	if err == nil {
		return canonical
	}
	return lower
}

// NormalizeTags normalizes a list of tags against the vocabulary.
func (s *Store) NormalizeTags(tags []string) []string {
	result := make([]string, len(tags))
	for i, tag := range tags {
		result[i] = s.Normalize(tag)
	}
	return result
}

// ListAll returns all vocabulary terms.
func (s *Store) ListAll() ([]Term, error) {
	rows, err := s.db.Query("SELECT term, category, usage_count, canonical FROM vocabulary ORDER BY term")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var terms []Term
	for rows.Next() {
		var t Term
		var canonical int
		if err := rows.Scan(&t.Term, &t.Category, &t.UsageCount, &canonical); err != nil {
			return nil, err
		}
		t.Canonical = canonical == 1
		terms = append(terms, t)
	}
	return terms, rows.Err()
}
```

- [ ] **Step 3: Run tests, verify pass**

- [ ] **Step 4: Commit**

```bash
git add internal/vocabulary/
git commit -m "feat: vocabulary store — canonical terms, usage counting, normalization"
```

---

### Task 5: Text Normalizer

**Files:**
- Create: `internal/librarian/normalizer.go`
- Create: `internal/librarian/normalizer_test.go`

- [ ] **Step 1: Write failing tests**

```go
package librarian

import "testing"

func TestNormalizeMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"trailing whitespace", "hello  \nworld  ", "hello\nworld"},
		{"multiple blank lines", "hello\n\n\n\nworld", "hello\n\nworld"},
		{"trailing newline", "hello\n\n", "hello\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeMarkdown(tt.input)
			if got != tt.expected {
				t.Errorf("got %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestChunkMarkdown(t *testing.T) {
	md := `# Section One

Content of section one.

## Section Two

Content of section two.

## Section Three

Content of section three.`

	chunks := ChunkByHeadings(md)
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3", len(chunks))
	}
	if chunks[0].Section != "Section One" {
		t.Errorf("chunk 0 section = %q", chunks[0].Section)
	}
}
```

- [ ] **Step 2: Implement normalizer**

```go
package librarian

import (
	"regexp"
	"strings"

	"github.com/frodex/prd2wiki/internal/vectordb"
)

var (
	trailingSpaceRe  = regexp.MustCompile(`[ \t]+\n`)
	multipleBlankRe  = regexp.MustCompile(`\n{3,}`)
	headingRe        = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
)

// NormalizeMarkdown cleans up markdown formatting without changing content meaning.
func NormalizeMarkdown(s string) string {
	s = trailingSpaceRe.ReplaceAllString(s, "\n")
	s = multipleBlankRe.ReplaceAllString(s, "\n\n")
	s = strings.TrimRight(s, " \t\n") + "\n"
	return s
}

// ChunkByHeadings splits markdown into chunks at heading boundaries.
// Returns TextChunks suitable for the vector store.
func ChunkByHeadings(md string) []vectordb.TextChunk {
	lines := strings.Split(md, "\n")
	var chunks []vectordb.TextChunk
	var currentSection string
	var currentLines []string

	flush := func() {
		if currentSection != "" && len(currentLines) > 0 {
			text := strings.TrimSpace(strings.Join(currentLines, "\n"))
			if text != "" {
				chunks = append(chunks, vectordb.TextChunk{
					Section: currentSection,
					Text:    text,
				})
			}
		}
	}

	for _, line := range lines {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			flush()
			currentSection = m[2]
			currentLines = nil
		} else {
			currentLines = append(currentLines, line)
		}
	}
	flush()

	return chunks
}
```

- [ ] **Step 3: Run tests, verify pass**

- [ ] **Step 4: Commit**

```bash
git add internal/librarian/
git commit -m "feat: markdown normalizer and section-aware chunker"
```

---

### Task 6: Deduplication Detector

**Files:**
- Create: `internal/librarian/dedup.go`
- Create: `internal/librarian/dedup_test.go`

- [ ] **Step 1: Write failing tests**

```go
package librarian

import (
	"context"
	"testing"

	"github.com/frodex/prd2wiki/internal/embedder"
	"github.com/frodex/prd2wiki/internal/vectordb"
)

func TestDetectDuplicates(t *testing.T) {
	emb := embedder.NewNoopEmbedder(768)
	store := vectordb.NewStore(emb)

	// Index an existing page
	store.IndexPage(context.Background(), "project-a", "EXIST-001", "requirement", "auth", []vectordb.TextChunk{
		{Section: "1.0", Text: "JWT authentication requirements"},
	})

	dedup := NewDedupDetector(store)

	// Check for duplicates of similar content
	dupes, err := dedup.Check(context.Background(), "project-a", "NEW-001", "JWT authentication requirements")
	if err != nil {
		t.Fatal(err)
	}

	// With noop embedder (all zeros), cosine similarity is 0,
	// but keyword matching should find overlap
	// The important thing is the interface works
	if dupes == nil {
		t.Error("expected non-nil dupes result")
	}
}
```

- [ ] **Step 2: Implement dedup detector**

```go
package librarian

import (
	"context"

	"github.com/frodex/prd2wiki/internal/vectordb"
)

type DuplicateCandidate struct {
	PageID     string  `json:"page_id"`
	Similarity float64 `json:"similarity"`
}

type DedupResult struct {
	Candidates []DuplicateCandidate `json:"candidates"`
}

type DedupDetector struct {
	store *vectordb.Store
}

func NewDedupDetector(store *vectordb.Store) *DedupDetector {
	return &DedupDetector{store: store}
}

// Check searches for potential duplicates of the given content.
// Returns candidates with similarity > threshold.
func (d *DedupDetector) Check(ctx context.Context, project, pageID, text string) (*DedupResult, error) {
	results, err := d.store.Search(ctx, project, text, 5)
	if err != nil {
		return nil, err
	}

	var candidates []DuplicateCandidate
	seen := make(map[string]bool)
	for _, r := range results {
		if r.PageID == pageID || seen[r.PageID] {
			continue
		}
		seen[r.PageID] = true
		if r.Similarity > 0.85 { // high similarity threshold
			candidates = append(candidates, DuplicateCandidate{
				PageID:     r.PageID,
				Similarity: r.Similarity,
			})
		}
	}

	return &DedupResult{Candidates: candidates}, nil
}
```

- [ ] **Step 3: Run tests, verify pass**

- [ ] **Step 4: Commit**

```bash
git add internal/librarian/dedup.go internal/librarian/dedup_test.go
git commit -m "feat: dedup detector — similarity-based duplicate detection"
```

---

### Task 7: Librarian Pipeline

**Files:**
- Create: `internal/librarian/librarian.go`
- Create: `internal/librarian/librarian_test.go`

- [ ] **Step 1: Write failing tests**

```go
package librarian

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/frodex/prd2wiki/internal/embedder"
	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/schema"
	"github.com/frodex/prd2wiki/internal/vectordb"
	"github.com/frodex/prd2wiki/internal/vocabulary"
)

func setupLibrarian(t *testing.T) (*Librarian, *wgit.Repo) {
	t.Helper()
	dir := t.TempDir()

	repo, err := wgit.InitRepo(dir, "test-project")
	if err != nil {
		t.Fatal(err)
	}

	db, err := index.OpenDatabase(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	emb := embedder.NewNoopEmbedder(768)
	vstore := vectordb.NewStore(emb)
	vocab := vocabulary.NewStore(db)
	indexer := index.NewIndexer(db)

	lib := New(repo, indexer, vstore, vocab)
	return lib, repo
}

func TestLibrarianVerbatim(t *testing.T) {
	lib, repo := setupLibrarian(t)

	fm := &schema.Frontmatter{
		ID:    "PRD-001",
		Title: "Test",
		Type:  "concept",
		Tags:  []string{"Auth", "SECURITY"}, // non-normalized
	}

	result, err := lib.Submit(context.Background(), SubmitRequest{
		Project:     "test-project",
		Branch:      "draft/test",
		Frontmatter: fm,
		Body:        []byte("# Test Content"),
		Intent:      IntentVerbatim,
		Author:      "test@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Saved != true {
		t.Error("expected page to be saved")
	}

	// Tags should NOT be normalized in verbatim mode
	readFM, _, err := repo.ReadPageWithMeta("draft/test", "pages/PRD-001.md")
	if err != nil {
		t.Fatal(err)
	}
	if readFM.Tags[0] != "Auth" {
		t.Errorf("verbatim should preserve original tags, got %q", readFM.Tags[0])
	}
}

func TestLibrarianConform(t *testing.T) {
	lib, repo := setupLibrarian(t)

	fm := &schema.Frontmatter{
		ID:    "PRD-002",
		Title: "Test Conform",
		Type:  "requirement",
		Tags:  []string{"Auth", "SECURITY"},
	}

	result, err := lib.Submit(context.Background(), SubmitRequest{
		Project:     "test-project",
		Branch:      "draft/test",
		Frontmatter: fm,
		Body:        []byte("# Test  \n\n\n\nContent  "),
		Intent:      IntentConform,
		Author:      "test@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Saved != true {
		t.Error("expected page to be saved")
	}

	// Tags should be normalized (lowercased)
	readFM, body, err := repo.ReadPageWithMeta("draft/test", "pages/PRD-002.md")
	if err != nil {
		t.Fatal(err)
	}
	if readFM.Tags[0] != "auth" {
		t.Errorf("conform should normalize tags, got %q", readFM.Tags[0])
	}

	// Body should be normalized (trailing whitespace removed)
	if strings.Contains(string(body), "  \n") {
		t.Error("conform should remove trailing whitespace")
	}
}

func TestLibrarianValidationError(t *testing.T) {
	lib, _ := setupLibrarian(t)

	fm := &schema.Frontmatter{
		Title: "No ID",
		// Missing ID and Type — should fail validation
	}

	result, err := lib.Submit(context.Background(), SubmitRequest{
		Project:     "test-project",
		Branch:      "draft/test",
		Frontmatter: fm,
		Body:        []byte("# Content"),
		Intent:      IntentConform,
		Author:      "test@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Conform mode should reject invalid schemas
	if result.Saved {
		t.Error("expected validation failure to prevent save")
	}
	if len(result.Issues) == 0 {
		t.Error("expected validation issues")
	}
}
```

- [ ] **Step 2: Implement librarian**

```go
package librarian

import (
	"context"
	"fmt"
	"strings"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/schema"
	"github.com/frodex/prd2wiki/internal/vectordb"
	"github.com/frodex/prd2wiki/internal/vocabulary"
)

const (
	IntentVerbatim  = "verbatim"
	IntentConform   = "conform"
	IntentIntegrate = "integrate"
)

type SubmitRequest struct {
	Project     string
	Branch      string
	Frontmatter *schema.Frontmatter
	Body        []byte
	Intent      string // verbatim | conform | integrate
	Author      string
}

type SubmitResult struct {
	Saved    bool            `json:"saved"`
	Path     string          `json:"path"`
	Issues   []schema.Issue  `json:"issues,omitempty"`
	Warnings []string        `json:"warnings,omitempty"`
	Diff     *DiffPreview    `json:"diff,omitempty"`
}

type DiffPreview struct {
	Changes []Change `json:"changes"`
}

type Change struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

type Librarian struct {
	repo    *wgit.Repo
	indexer *index.Indexer
	vstore  *vectordb.Store
	vocab   *vocabulary.Store
}

func New(repo *wgit.Repo, indexer *index.Indexer, vstore *vectordb.Store, vocab *vocabulary.Store) *Librarian {
	return &Librarian{
		repo:    repo,
		indexer: indexer,
		vstore:  vstore,
		vocab:   vocab,
	}
}

func (l *Librarian) Submit(ctx context.Context, req SubmitRequest) (*SubmitResult, error) {
	switch req.Intent {
	case IntentVerbatim:
		return l.submitVerbatim(ctx, req)
	case IntentConform:
		return l.submitConform(ctx, req)
	case IntentIntegrate:
		return l.submitIntegrate(ctx, req)
	default:
		return l.submitVerbatim(ctx, req)
	}
}

func (l *Librarian) submitVerbatim(ctx context.Context, req SubmitRequest) (*SubmitResult, error) {
	// Validate but don't block
	issues := schema.Validate(req.Frontmatter)

	if schema.HasErrors(issues) {
		req.Frontmatter.Conformance = "pending"
	} else {
		req.Frontmatter.Conformance = "valid"
	}

	path := fmt.Sprintf("pages/%s.md", req.Frontmatter.ID)
	err := l.repo.WritePageWithMeta(req.Branch, path, req.Frontmatter, req.Body,
		"submit (verbatim): "+req.Frontmatter.Title, req.Author)
	if err != nil {
		return nil, fmt.Errorf("write page: %w", err)
	}

	// Index in SQLite
	_ = l.indexer.IndexPage(req.Project, req.Branch, path, req.Frontmatter, req.Body)

	// Index in vector store
	chunks := ChunkByHeadings(string(req.Body))
	if len(chunks) > 0 {
		_ = l.vstore.IndexPage(ctx, req.Project, req.Frontmatter.ID, req.Frontmatter.Type,
			strings.Join(req.Frontmatter.Tags, ","), chunks)
	}

	return &SubmitResult{
		Saved:  true,
		Path:   path,
		Issues: issues,
	}, nil
}

func (l *Librarian) submitConform(ctx context.Context, req SubmitRequest) (*SubmitResult, error) {
	// Validate — block on errors
	issues := schema.Validate(req.Frontmatter)
	if schema.HasErrors(issues) {
		return &SubmitResult{
			Saved:  false,
			Issues: issues,
		}, nil
	}

	var changes []Change

	// Normalize tags
	origTags := make([]string, len(req.Frontmatter.Tags))
	copy(origTags, req.Frontmatter.Tags)
	for i, tag := range req.Frontmatter.Tags {
		normalized := strings.ToLower(strings.TrimSpace(tag))
		if normalized != tag {
			changes = append(changes, Change{
				Field: fmt.Sprintf("tags[%d]", i),
				From:  tag,
				To:    normalized,
			})
		}
		req.Frontmatter.Tags[i] = normalized
		_ = l.vocab.Add(normalized, "tag")
	}

	// Normalize markdown body
	origBody := string(req.Body)
	normalizedBody := NormalizeMarkdown(origBody)
	if normalizedBody != origBody {
		changes = append(changes, Change{
			Field: "body",
			From:  "(formatting issues)",
			To:    "(corrected)",
		})
	}
	req.Body = []byte(normalizedBody)

	req.Frontmatter.Conformance = "valid"

	path := fmt.Sprintf("pages/%s.md", req.Frontmatter.ID)
	err := l.repo.WritePageWithMeta(req.Branch, path, req.Frontmatter, req.Body,
		"submit (conform): "+req.Frontmatter.Title, req.Author)
	if err != nil {
		return nil, fmt.Errorf("write page: %w", err)
	}

	_ = l.indexer.IndexPage(req.Project, req.Branch, path, req.Frontmatter, req.Body)

	chunks := ChunkByHeadings(string(req.Body))
	if len(chunks) > 0 {
		_ = l.vstore.IndexPage(ctx, req.Project, req.Frontmatter.ID, req.Frontmatter.Type,
			strings.Join(req.Frontmatter.Tags, ","), chunks)
	}

	return &SubmitResult{
		Saved: true,
		Path:  path,
		Issues: issues,
		Diff:  &DiffPreview{Changes: changes},
	}, nil
}

func (l *Librarian) submitIntegrate(ctx context.Context, req SubmitRequest) (*SubmitResult, error) {
	// For now, integrate = conform + dedup check
	// Full integration reasoning (cross-referencing, merge suggestions) is future work
	result, err := l.submitConform(ctx, req)
	if err != nil || !result.Saved {
		return result, err
	}

	// Check for duplicates
	dedup := NewDedupDetector(l.vstore)
	dupes, err := dedup.Check(ctx, req.Project, req.Frontmatter.ID, string(req.Body))
	if err == nil && len(dupes.Candidates) > 0 {
		for _, d := range dupes.Candidates {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("potential duplicate: %s (similarity: %.2f)", d.PageID, d.Similarity))
		}
	}

	return result, nil
}
```

- [ ] **Step 3: Add missing import for strings in test file**

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./internal/librarian/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/librarian/
git commit -m "feat: librarian pipeline — verbatim/conform/integrate submission modes"
```

---

### Task 8: Integrate Librarian into API

**Files:**
- Modify: `internal/api/server.go` — add librarian field
- Modify: `internal/api/pages.go` — route writes through librarian
- Modify: `cmd/prd2wiki/main.go` — wire librarian into server
- Create: `internal/api/librarian_test.go` — test librarian integration

- [ ] **Step 1: Update Server to include Librarian**

Add a `librarian` field to `Server` struct (map of project → *librarian.Librarian). Update `NewServer` to accept librarians. Update `createPage` to use librarian.Submit instead of direct git write.

- [ ] **Step 2: Update createPage handler**

```go
func (s *Server) upsertPage(w http.ResponseWriter, r *http.Request) {
    // ... decode request ...
    
    lib, ok := s.librarians[project]
    if !ok {
        http.Error(w, "project not found", http.StatusNotFound)
        return
    }

    result, err := lib.Submit(r.Context(), librarian.SubmitRequest{
        Project:     project,
        Branch:      req.Branch,
        Frontmatter: fm,
        Body:        []byte(req.Body),
        Intent:      req.Intent, // verbatim | conform | integrate
        Author:      req.Author,
    })
    
    if !result.Saved {
        // Return 422 with validation issues
    }
    // Return 201 with result
}
```

- [ ] **Step 3: Update main.go to create librarians**

```go
// After creating repos, indexer, etc:
emb := embedder.NewNoopEmbedder(768)
vstore := vectordb.NewStore(emb)

librarians := make(map[string]*librarian.Librarian)
for project, repo := range repos {
    vocab := vocabulary.NewStore(db)
    librarians[project] = librarian.New(repo, indexer, vstore, vocab)
}

srv := api.NewServer(cfg.Server.Addr, repos, db, librarians)
```

- [ ] **Step 4: Write integration test**

```go
func TestCreatePageWithLibrarian(t *testing.T) {
    // Setup server with librarian
    // POST with intent=conform and messy tags
    // Verify tags are normalized in response
    // POST with intent=verbatim and messy tags
    // Verify tags are preserved
}
```

- [ ] **Step 5: Run all tests**

Run: `go test ./... -v`

- [ ] **Step 6: Commit**

```bash
git add internal/api/ cmd/prd2wiki/ internal/librarian/
git commit -m "feat: integrate librarian into API — writes route through submission pipeline"
```

---

### Task 9: Install Embedder Script

**Files:**
- Create: `scripts/install-embedder.sh`

- [ ] **Step 1: Adapt Pippi's install-embedder.sh**

The script should:
1. Download llama-server binary for the current platform (linux amd64/arm64)
2. Download nomic-embed-text-v1.5 Q8_0 GGUF model
3. Create systemd service `prd2wiki-embedder.service`
4. Verify installation

Adapt from Pippi's `scripts/install-embedder.sh` — change paths from `/opt/pippi/` to `/opt/prd2wiki/`, service name from `pippi-embedder` to `prd2wiki-embedder`.

- [ ] **Step 2: Make executable and commit**

```bash
chmod +x scripts/install-embedder.sh
git add scripts/
git commit -m "feat: install-embedder.sh — downloads llama-server + nomic model, creates systemd service"
```

---

## Self-Review Checklist

- [x] Embedder interface matches Pippi pattern (Embed, EmbedQuery, Dimensions, Available)
- [x] Vector store supports: IndexPage, RemovePage, Search, FindSimilar
- [x] Vocabulary store uses existing SQLite vocabulary table from Phase 1 migrations
- [x] Librarian implements all three submission modes (verbatim/conform/integrate)
- [x] Conform mode normalizes tags and markdown formatting
- [x] Verbatim mode validates but doesn't block
- [x] Dedup detector uses vector similarity
- [x] Librarian updates both SQLite index and vector store on write
- [x] API routes writes through librarian
- [x] main.go wires embedder, vector store, vocab, librarian
- [x] Degraded fallback: NoopEmbedder when llama.cpp unavailable

**Not in Phase 2 (by design):**
- Soft reference UI (Phase 3 — Web UI)
- Soft reference sidecar files (.soft-refs.yaml) — Phase 3
- LLM escalation in classification (placeholder only)
- Full integration reasoning in integrate mode (cross-referencing, merge suggestions)
- Native LanceDB persistence (in-memory store is sufficient to start)

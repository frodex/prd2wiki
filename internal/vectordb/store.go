package vectordb

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/frodex/prd2wiki/internal/embedder"
)

// Store is a vector store for page embeddings with optional disk persistence.
// When a persist path is configured, the store auto-saves after every write.
type Store struct {
	mu          sync.RWMutex
	records     []PageEmbedding
	emb         embedder.Embedder
	persistPath string // if set, auto-save on every write
}

// NewStore creates a new Store using the provided Embedder.
func NewStore(emb embedder.Embedder) *Store {
	return &Store{emb: emb}
}

// SetPersistPath configures auto-save: after every IndexPage or RemovePage,
// the full record set is written to this path as JSON.
func (s *Store) SetPersistPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.persistPath = path
}

// Count returns the number of embedding records in the store.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

// IndexPage embeds all chunks, removes existing entries for pageID, and stores new records.
func (s *Store) IndexPage(ctx context.Context, project, pageID, typ, tags string, chunks []TextChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}

	vectors, err := s.emb.EmbedBatch(ctx, texts, embedder.DefaultLanguage)
	if err != nil {
		return err
	}

	records := make([]PageEmbedding, len(chunks))
	for i, c := range chunks {
		records[i] = PageEmbedding{
			PageID:  pageID,
			Section: c.Section,
			Project: project,
			Type:    typ,
			Tags:    tags,
			Vector:  vectors[i],
			Text:    c.Text,
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing entries for this pageID.
	filtered := s.records[:0:0]
	for _, r := range s.records {
		if r.PageID != pageID {
			filtered = append(filtered, r)
		}
	}
	s.records = append(filtered, records...)

	if err := s.saveLocked(); err != nil {
		return fmt.Errorf("persist after IndexPage: %w", err)
	}

	return nil
}

// RemovePage removes all records associated with pageID.
func (s *Store) RemovePage(pageID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := s.records[:0:0]
	for _, r := range s.records {
		if r.PageID != pageID {
			filtered = append(filtered, r)
		}
	}
	s.records = filtered
	_ = s.saveLocked() // best-effort persist
}

// SaveToDisk writes the current records to path as JSON.
func (s *Store) SaveToDisk(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return writeJSON(path, s.records)
}

// LoadFromDisk reads records from a JSON file. Existing records are replaced.
func (s *Store) LoadFromDisk(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var records []PageEmbedding
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = records
	return nil
}

// saveLocked writes records to persistPath. Caller must hold s.mu (read or write).
func (s *Store) saveLocked() error {
	if s.persistPath == "" {
		return nil
	}
	return writeJSON(s.persistPath, s.records)
}

// writeJSON atomically writes data as JSON to path (write to temp, rename).
func writeJSON(path string, v any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	tmp := path + ".tmp"
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

type scoredResult struct {
	result SearchResult
	score  float64
}

// Search embeds the query, computes fused cosine + keyword scores, and returns the top N results.
func (s *Store) Search(ctx context.Context, project, query string, limit int) ([]SearchResult, error) {
	queryVec, err := s.emb.EmbedQuery(ctx, query, embedder.DefaultLanguage)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	records := make([]PageEmbedding, len(s.records))
	copy(records, s.records)
	s.mu.RUnlock()

	var scored []scoredResult
	for _, r := range records {
		if r.Project != project {
			continue
		}
		cosine := cosineSimilarity(queryVec, r.Vector)
		kw := keywordScore(query, r.Text)
		fused := 0.7*cosine + 0.3*kw
		scored = append(scored, scoredResult{
			result: SearchResult{
				PageID:     r.PageID,
				Section:    r.Section,
				Similarity: fused,
				Text:       r.Text,
			},
			score: fused,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if limit > 0 && len(scored) > limit {
		scored = scored[:limit]
	}

	results := make([]SearchResult, len(scored))
	for i, s := range scored {
		results[i] = s.result
	}
	return results, nil
}

// FindSimilar finds pages similar to pageID by searching with the first chunk's text,
// filtering out pageID itself.
func (s *Store) FindSimilar(ctx context.Context, project, pageID string, limit int) ([]SearchResult, error) {
	s.mu.RLock()
	var firstText string
	for _, r := range s.records {
		if r.PageID == pageID && r.Project == project {
			firstText = r.Text
			break
		}
	}
	s.mu.RUnlock()

	if firstText == "" {
		return nil, nil
	}

	// Search with a higher limit to account for filtering out pageID.
	searchLimit := limit + 1
	results, err := s.Search(ctx, project, firstText, searchLimit)
	if err != nil {
		return nil, err
	}

	filtered := results[:0:0]
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

// cosineSimilarity computes the cosine similarity between two float32 vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// keywordScore returns the fraction of unique query words found in text (case-insensitive).
func keywordScore(query, text string) float64 {
	queryWords := strings.Fields(strings.ToLower(query))
	if len(queryWords) == 0 {
		return 0
	}
	lowerText := strings.ToLower(text)
	seen := make(map[string]bool)
	var matched int
	for _, w := range queryWords {
		if seen[w] {
			continue
		}
		seen[w] = true
		if strings.Contains(lowerText, w) {
			matched++
		}
	}
	return float64(matched) / float64(len(seen))
}

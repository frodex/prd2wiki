package librarian

import (
	"context"
	"fmt"

	"github.com/frodex/prd2wiki/internal/vectordb"
)

// DuplicateCandidate represents a page that may be a duplicate.
type DuplicateCandidate struct {
	PageID     string  `json:"page_id"`
	Similarity float64 `json:"similarity"`
}

// DedupResult holds the result of a dedup check.
type DedupResult struct {
	Candidates []DuplicateCandidate `json:"candidates"`
}

// DedupDetector checks for duplicate content in the vector store.
type DedupDetector struct {
	store *vectordb.Store
}

// NewDedupDetector creates a new DedupDetector backed by the given store.
func NewDedupDetector(store *vectordb.Store) *DedupDetector {
	return &DedupDetector{store: store}
}

// Check searches for pages similar to the given text in the vector store.
// It returns candidates with similarity > 0.85, excluding the page itself.
func (d *DedupDetector) Check(ctx context.Context, project, pageID, text string) (*DedupResult, error) {
	results, err := d.store.Search(ctx, project, text, 5)
	if err != nil {
		return nil, fmt.Errorf("dedup search: %w", err)
	}

	var candidates []DuplicateCandidate
	seen := make(map[string]bool)
	for _, r := range results {
		if r.PageID == pageID {
			continue
		}
		if r.Similarity <= 0.85 {
			continue
		}
		if seen[r.PageID] {
			continue
		}
		seen[r.PageID] = true
		candidates = append(candidates, DuplicateCandidate{
			PageID:     r.PageID,
			Similarity: r.Similarity,
		})
	}

	return &DedupResult{Candidates: candidates}, nil
}

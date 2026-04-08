package vocabulary

import (
	"database/sql"
	"fmt"
	"strings"
)

// Term represents a vocabulary entry.
type Term struct {
	Term       string `json:"term"`
	Category   string `json:"category"`
	UsageCount int    `json:"usage_count"`
	Canonical  bool   `json:"canonical"`
}

// Store provides vocabulary operations backed by SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a new Store using the provided database connection.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Add upserts the term (lowercased) into the vocabulary, incrementing usage_count on conflict.
func (s *Store) Add(term, category string) error {
	lower := strings.ToLower(term)
	_, err := s.db.Exec(`
		INSERT INTO vocabulary (term, category, usage_count, canonical)
		VALUES (?, ?, 1, 1)
		ON CONFLICT(term) DO UPDATE SET
			usage_count = usage_count + 1,
			category    = excluded.category
	`, lower, category)
	if err != nil {
		return fmt.Errorf("vocabulary add %q: %w", lower, err)
	}
	return nil
}

// Get returns the Term for an exact (lowercased) match, or nil if not found.
func (s *Store) Get(term string) (*Term, error) {
	lower := strings.ToLower(term)
	var t Term
	var canonical int
	err := s.db.QueryRow(`
		SELECT term, category, usage_count, canonical
		FROM vocabulary
		WHERE term = ?
	`, lower).Scan(&t.Term, &t.Category, &t.UsageCount, &canonical)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("vocabulary get %q: %w", lower, err)
	}
	t.Canonical = canonical != 0
	return &t, nil
}

// Normalize lowercases the input, checks the vocabulary, and returns the canonical form.
// If the term is not in the vocabulary it is returned lowercased as-is.
func (s *Store) Normalize(term string) string {
	lower := strings.ToLower(term)
	t, err := s.Get(lower)
	if err != nil || t == nil {
		return lower
	}
	return t.Term
}

// NormalizeTags normalizes each tag in the slice and returns a new slice.
func (s *Store) NormalizeTags(tags []string) []string {
	out := make([]string, len(tags))
	for i, tag := range tags {
		out[i] = s.Normalize(tag)
	}
	return out
}

// ListAll returns all vocabulary terms sorted alphabetically by term.
func (s *Store) ListAll() ([]Term, error) {
	rows, err := s.db.Query(`
		SELECT term, category, usage_count, canonical
		FROM vocabulary
		ORDER BY term ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("vocabulary list: %w", err)
	}
	defer rows.Close()

	var terms []Term
	for rows.Next() {
		var t Term
		var canonical int
		if err := rows.Scan(&t.Term, &t.Category, &t.UsageCount, &canonical); err != nil {
			return nil, fmt.Errorf("vocabulary scan: %w", err)
		}
		t.Canonical = canonical != 0
		terms = append(terms, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vocabulary rows: %w", err)
	}
	return terms, nil
}

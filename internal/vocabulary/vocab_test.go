package vocabulary_test

import (
	"path/filepath"
	"testing"

	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/vocabulary"
)

func openTestDB(t *testing.T) *vocabulary.Store {
	t.Helper()
	dir := t.TempDir()
	db, err := index.OpenDatabase(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDatabase: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return vocabulary.NewStore(db)
}

func TestAddAndGetTerm(t *testing.T) {
	s := openTestDB(t)

	if err := s.Add("API", "technology"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	term, err := s.Get("api")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if term == nil {
		t.Fatal("expected term, got nil")
	}
	if term.Term != "api" {
		t.Errorf("Term = %q, want %q", term.Term, "api")
	}
	if term.Category != "technology" {
		t.Errorf("Category = %q, want %q", term.Category, "technology")
	}
	if term.UsageCount != 1 {
		t.Errorf("UsageCount = %d, want 1", term.UsageCount)
	}
	if !term.Canonical {
		t.Error("Canonical = false, want true")
	}
}

func TestUsageCount(t *testing.T) {
	s := openTestDB(t)

	for i := 0; i < 3; i++ {
		if err := s.Add("REST", "technology"); err != nil {
			t.Fatalf("Add iteration %d: %v", i, err)
		}
	}

	term, err := s.Get("rest")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if term.UsageCount != 3 {
		t.Errorf("UsageCount = %d, want 3", term.UsageCount)
	}
}

func TestNormalize(t *testing.T) {
	s := openTestDB(t)

	// Add a canonical term
	if err := s.Add("GraphQL", "technology"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{"GraphQL", "graphql"},   // case folding
		{"graphql", "graphql"},   // already lowercase, in vocab
		{"GRAPHQL", "graphql"},   // all caps
		{"Unknown", "unknown"},   // not in vocab, still lowercased
	}

	for _, tc := range tests {
		got := s.Normalize(tc.input)
		if got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeTags(t *testing.T) {
	s := openTestDB(t)

	if err := s.Add("DevOps", "practice"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	tags := []string{"DevOps", "REST", "API"}
	got := s.NormalizeTags(tags)

	want := []string{"devops", "rest", "api"}
	if len(got) != len(want) {
		t.Fatalf("NormalizeTags len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("NormalizeTags[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestListAll(t *testing.T) {
	s := openTestDB(t)

	terms := []struct{ term, cat string }{
		{"Zebra", "animal"},
		{"Apple", "fruit"},
		{"Mango", "fruit"},
	}
	for _, tc := range terms {
		if err := s.Add(tc.term, tc.cat); err != nil {
			t.Fatalf("Add %q: %v", tc.term, err)
		}
	}

	all, err := s.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ListAll len = %d, want 3", len(all))
	}
	// Verify sorted order
	if all[0].Term != "apple" {
		t.Errorf("all[0].Term = %q, want %q", all[0].Term, "apple")
	}
	if all[1].Term != "mango" {
		t.Errorf("all[1].Term = %q, want %q", all[1].Term, "mango")
	}
	if all[2].Term != "zebra" {
		t.Errorf("all[2].Term = %q, want %q", all[2].Term, "zebra")
	}
}

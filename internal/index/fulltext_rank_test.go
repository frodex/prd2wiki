package index

import "testing"

func TestNormalizeForTitleMatch(t *testing.T) {
	got := normalizeForTitleMatch("DRAFT: pippi-librarian README.md")
	want := "draft pippi librarian readme md"
	if got != want {
		t.Fatalf("normalizeForTitleMatch: got %q want %q", got, want)
	}
}

func TestMatchTier(t *testing.T) {
	title := "DRAFT: pippi-librarian README.md"
	tags := "pippi,librarian,readme,documentation"
	if g := MatchTier(title, tags, "pippi readme"); g != 1 {
		t.Fatalf("both terms in title: got %d want 1", g)
	}
	if g := MatchTier(title, tags, "pippi librarian readme"); g != 0 {
		t.Fatalf("phrase in blob: got %d want 0", g)
	}
	if g := MatchTier("MANIFEST: pippi-librarian Standalone", "pippi,librarian", "pippi readme"); g != 3 {
		t.Fatalf("missing readme: got %d want 3", g)
	}
	// Terms only via tags (non-adjacent in blob) → tier 2
	if g := MatchTier("alpha beta", "pippi,extra,readme", "pippi readme"); g != 2 {
		t.Fatalf("tags-only match: got %d want 2", g)
	}
}

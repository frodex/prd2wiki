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

func TestTitleMatchBonus(t *testing.T) {
	if g := TitleMatchBonus("Agent Rules Bootstrap", "agent rules bootstrap"); g < 500 {
		t.Fatalf("exact title: got %g want >= 500", g)
	}
	if g := TitleMatchBonus("Agent Rules Bootstrap — notes", "agent rules bootstrap"); g < 400 {
		t.Fatalf("prefix title: got %g want >= 400", g)
	}
	if TitleMatchBonus("Other", "agent rules bootstrap") != 0 {
		t.Fatal("unrelated title should be 0")
	}
	// “Research: …” contains phrase but should score lower than a title that *starts* with the query.
	prefix := TitleMatchBonus("Composable Pages — Live Edits", "composable pages")
	mid := TitleMatchBonus("Research: Composable Pages Design", "composable pages")
	if prefix <= mid {
		t.Fatalf("prefix title should beat mid-title phrase: prefix=%g mid=%g", prefix, mid)
	}
}

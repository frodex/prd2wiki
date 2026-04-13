package index

import "testing"

func TestNormalizeForTitleMatch(t *testing.T) {
	got := normalizeForTitleMatch("DRAFT: pippi-librarian README.md")
	want := "draft pippi librarian readme md"
	if got != want {
		t.Fatalf("normalizeForTitleMatch: got %q want %q", got, want)
	}
}

func TestFullTextTitleTier(t *testing.T) {
	title := "DRAFT: pippi-librarian README.md"
	if g := fullTextTitleTier(title, "pippi readme"); g != 1 {
		t.Fatalf("tier for pippi readme: got %d want 1 (all tokens in normalized title)", g)
	}
	if g := fullTextTitleTier(title, "pippi librarian readme"); g != 0 {
		t.Fatalf("phrase tier: got %d want 0", g)
	}
	if g := fullTextTitleTier("MANIFEST: pippi-librarian Standalone", "pippi readme"); g != 2 {
		t.Fatalf("missing readme in title: got %d want 2", g)
	}
}

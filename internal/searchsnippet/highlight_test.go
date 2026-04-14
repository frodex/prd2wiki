package searchsnippet

import (
	"strings"
	"testing"
)

func TestHighlightPlainAsHTML_escapesAndMarks(t *testing.T) {
	plain := `Say "todo" here and TODO again.`
	q := `todo`
	got := string(HighlightPlainAsHTML(plain, q))
	if !strings.Contains(got, `<mark class="search-hit">`) {
		t.Fatalf("missing mark: %s", got)
	}
	if strings.Contains(got, `<script`) {
		t.Fatalf("unexpected script")
	}
	// Both case variants marked
	if c := strings.Count(got, "<mark"); c < 2 {
		t.Fatalf("want 2+ marks, got %d: %s", c, got)
	}
}

func TestFormatSearchExcerpt_clampsThenMarks(t *testing.T) {
	snip := "needle in body " + strings.Repeat("word ", 80)
	q := "needle"
	got := string(FormatSearchExcerpt(snip, q))
	if !strings.Contains(got, "needle") {
		t.Fatalf("lost needle: %s", got)
	}
	if !strings.Contains(got, "<mark") {
		t.Fatalf("no mark: %s", got)
	}
	if len([]rune(got)) > 400 {
		t.Fatalf("excerpt too long: %d", len([]rune(got)))
	}
}

func TestQueryTermsForHighlight_skipsNoise(t *testing.T) {
	ts := queryTermsForHighlight(`foo OR bar AND "baz"`)
	if len(ts) != 3 {
		t.Fatalf("got %v", ts)
	}
}

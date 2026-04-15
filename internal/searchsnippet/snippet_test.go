package searchsnippet

import (
	"strings"
	"testing"
)

func TestClampExcerpt_linesAndRunes(t *testing.T) {
	s := "line one\nline two\nline three\nmore"
	got := ClampExcerpt(s, 100, 2)
	if want := "line one\nline two"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	long := strings.Repeat("x", 250)
	got = ClampExcerpt(long, 200, 0)
	if n := len([]rune(got)); n != 201 { // 200 + ellipsis
		t.Fatalf("runes: got %d", n)
	}
}

func TestHistoryVectorExcerpt(t *testing.T) {
	got := HistoryVectorExcerpt("a1b2c3d4e5f6", "needle in past")
	if want := "[history]a1b2c3d4e5f6 — needle in past"; got != want {
		t.Fatalf("got %q", got)
	}
}

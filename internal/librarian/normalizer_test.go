package librarian_test

import (
	"strings"
	"testing"

	"github.com/frodex/prd2wiki/internal/librarian"
)

func TestNormalizeMarkdown(t *testing.T) {
	t.Run("removes trailing whitespace from lines", func(t *testing.T) {
		input := "hello   \nworld  \n"
		got := librarian.NormalizeMarkdown(input)
		lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
		if lines[0] != "hello" {
			t.Errorf("line 0 = %q, want %q", lines[0], "hello")
		}
		if lines[1] != "world" {
			t.Errorf("line 1 = %q, want %q", lines[1], "world")
		}
	})

	t.Run("collapses 3+ blank lines to 2", func(t *testing.T) {
		input := "a\n\n\n\n\nb\n"
		got := librarian.NormalizeMarkdown(input)
		// Should have at most 2 consecutive blank lines (3 consecutive newlines)
		// \n\n\n\n would mean 3+ blank lines, which is not allowed
		if strings.Contains(got, "\n\n\n\n") {
			t.Errorf("output still contains 3+ blank lines: %q", got)
		}
		// Should still have content separated by exactly 2 blank lines
		want := "a\n\n\nb\n"
		if got != want {
			t.Errorf("NormalizeMarkdown() = %q, want %q", got, want)
		}
	})

	t.Run("ensures single trailing newline", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
		}{
			{"no trailing newline", "content"},
			{"multiple trailing newlines", "content\n\n\n"},
			{"single trailing newline already", "content\n"},
		}
		for _, tc := range tests {
			got := librarian.NormalizeMarkdown(tc.input)
			if !strings.HasSuffix(got, "\n") {
				t.Errorf("[%s] output does not end with newline: %q", tc.name, got)
			}
			trimmed := strings.TrimRight(got, "\n")
			if strings.HasSuffix(trimmed, "\n") {
				t.Errorf("[%s] output has multiple trailing newlines: %q", tc.name, got)
			}
		}
	})
}


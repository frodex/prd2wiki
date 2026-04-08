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

func TestChunkByHeadings(t *testing.T) {
	md := `# Introduction

This is the intro.

## Background

Some background here.

### Details

More details.
`
	chunks := librarian.ChunkByHeadings(md)

	if len(chunks) != 3 {
		t.Fatalf("ChunkByHeadings returned %d chunks, want 3", len(chunks))
	}

	wantSections := []string{"Introduction", "Background", "Details"}
	for i, want := range wantSections {
		if chunks[i].Section != want {
			t.Errorf("chunk[%d].Section = %q, want %q", i, chunks[i].Section, want)
		}
	}

	// Verify text content is present under headings
	if !strings.Contains(chunks[0].Text, "intro") {
		t.Errorf("chunk[0].Text = %q, expected to contain 'intro'", chunks[0].Text)
	}
	if !strings.Contains(chunks[1].Text, "background") {
		t.Errorf("chunk[1].Text = %q, expected to contain 'background'", chunks[1].Text)
	}
	if !strings.Contains(chunks[2].Text, "details") {
		t.Errorf("chunk[2].Text = %q, expected to contain 'details'", chunks[2].Text)
	}
}

func TestChunkByHeadingsEmptyDoc(t *testing.T) {
	chunks := librarian.ChunkByHeadings("")
	if len(chunks) != 0 {
		t.Errorf("ChunkByHeadings(\"\") = %d chunks, want 0", len(chunks))
	}
}

func TestChunkByHeadingsNoHeadings(t *testing.T) {
	md := "Just some text with no headings.\n"
	chunks := librarian.ChunkByHeadings(md)
	if len(chunks) != 0 {
		t.Errorf("ChunkByHeadings with no headings returned %d chunks, want 0", len(chunks))
	}
}

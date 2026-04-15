package librarian

import (
	"regexp"
	"strings"
)

var (
	trailingSpaceRe = regexp.MustCompile(`[ \t]+(\n|$)`)
	threeBlankRe    = regexp.MustCompile(`\n{4,}`) // 4+ newlines = 3+ blank lines
)

// NormalizeMarkdown cleans up a markdown string:
//   - Removes trailing whitespace from each line.
//   - Collapses 3 or more consecutive blank lines into exactly 2.
//   - Ensures the output ends with a single newline.
func NormalizeMarkdown(s string) string {
	// Remove trailing whitespace from each line (preserving newlines)
	s = trailingSpaceRe.ReplaceAllString(s, "$1")

	// Collapse 4+ newlines (3+ blank lines) to exactly 3 newlines (2 blank lines)
	s = threeBlankRe.ReplaceAllString(s, "\n\n\n")

	// Ensure single trailing newline
	s = strings.TrimRight(s, "\n")
	s += "\n"

	return s
}


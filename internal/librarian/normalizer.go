package librarian

import (
	"regexp"
	"strings"

	"github.com/frodex/prd2wiki/internal/vectordb"
)

var (
	trailingSpaceRe = regexp.MustCompile(`[ \t]+(\n|$)`)
	threeBlankRe    = regexp.MustCompile(`\n{4,}`) // 4+ newlines = 3+ blank lines
	headingRe       = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)
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

// ChunkByHeadings splits markdown into TextChunk values at each heading boundary.
// The Section field contains the heading text; Text contains the content that follows.
// Content before the first heading is discarded.
func ChunkByHeadings(md string) []vectordb.TextChunk {
	if md == "" {
		return nil
	}

	lines := strings.Split(md, "\n")
	var chunks []vectordb.TextChunk

	currentSection := ""
	var currentLines []string
	inSection := false

	for _, line := range lines {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			// Save previous section if we had one
			if inSection {
				chunks = append(chunks, vectordb.TextChunk{
					Section: currentSection,
					Text:    strings.TrimSpace(strings.Join(currentLines, "\n")),
				})
			}
			currentSection = strings.TrimSpace(m[2])
			currentLines = nil
			inSection = true
		} else if inSection {
			currentLines = append(currentLines, line)
		}
	}

	// Flush last section
	if inSection {
		chunks = append(chunks, vectordb.TextChunk{
			Section: currentSection,
			Text:    strings.TrimSpace(strings.Join(currentLines, "\n")),
		})
	}

	return chunks
}

package index

import (
	"strings"
	"unicode"
)

// fullTextTitleTier classifies how well a page title matches a free-text query.
// Lower tier values sort first (more relevant). Used after FTS retrieval.
func fullTextTitleTier(title, query string) int {
	t := normalizeForTitleMatch(title)
	q := normalizeForTitleMatch(query)
	if t == "" || q == "" {
		return 2
	}
	if strings.Contains(t, q) {
		return 0
	}
	for _, tok := range strings.Fields(q) {
		r := []rune(tok)
		if len(r) < 2 {
			continue
		}
		if !strings.Contains(t, tok) {
			return 2
		}
	}
	return 1
}

func normalizeForTitleMatch(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := true
	for _, r := range s {
		if r == '-' || r == '_' || unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		// drop punctuation (e.g. ":" in "DRAFT:")
		if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

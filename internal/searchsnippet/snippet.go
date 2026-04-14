package searchsnippet

import "strings"

// ClampExcerpt trims to at most maxLines newlines and maxRunes runes.
func ClampExcerpt(s string, maxRunes, maxLines int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if maxLines > 0 {
		parts := strings.Split(s, "\n")
		if len(parts) > maxLines {
			s = strings.Join(parts[:maxLines], "\n")
		}
	}
	if maxRunes > 0 {
		r := []rune(s)
		if len(r) > maxRunes {
			return string(r[:maxRunes]) + "…"
		}
	}
	return s
}

// ExcerptContainsTerm checks if an HTML excerpt contains any of the query terms (case-insensitive).
func ExcerptContainsTerm(htmlExcerpt, query string) bool {
	lower := strings.ToLower(htmlExcerpt)
	for _, t := range strings.Fields(strings.ToLower(query)) {
		t = strings.Trim(t, `"'`)
		t = strings.TrimSuffix(t, "*")
		if len(t) < 2 {
			continue
		}
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

// FallbackExcerpt extracts context around the first case-insensitive occurrence of query in body.
// Used when FTS snippet() returns text that doesn't contain the search term.
func FallbackExcerpt(body, query string, contextRunes int) string {
	if body == "" || query == "" {
		return ""
	}
	lower := strings.ToLower(body)
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return ""
	}
	// Find first matching term
	bestIdx := -1
	for _, t := range terms {
		t = strings.Trim(t, `"'`)
		t = strings.TrimSuffix(t, "*")
		if len(t) < 2 {
			continue
		}
		idx := strings.Index(lower, t)
		if idx >= 0 && (bestIdx < 0 || idx < bestIdx) {
			bestIdx = idx
		}
	}
	if bestIdx < 0 {
		return ""
	}
	runes := []rune(body)
	runeIdx := len([]rune(body[:bestIdx]))
	start := runeIdx - contextRunes
	if start < 0 {
		start = 0
	}
	end := runeIdx + contextRunes
	if end > len(runes) {
		end = len(runes)
	}
	excerpt := string(runes[start:end])
	// Clean up: trim to line boundaries if possible
	if start > 0 {
		if nl := strings.Index(excerpt, "\n"); nl >= 0 && nl < contextRunes/2 {
			excerpt = excerpt[nl+1:]
		}
		excerpt = "… " + excerpt
	}
	if end < len(runes) {
		if nl := strings.LastIndex(excerpt, "\n"); nl >= 0 && nl > len([]rune(excerpt))-contextRunes/2 {
			excerpt = excerpt[:nl]
		}
		excerpt = excerpt + " …"
	}
	return strings.TrimSpace(excerpt)
}

// HistoryVectorExcerpt formats a deep-search hit against an older git revision.
func HistoryVectorExcerpt(shortCommit, vectorSnippet string) string {
	snip := ClampExcerpt(vectorSnippet, 200, 2)
	if shortCommit != "" {
		return "[history]" + shortCommit + " — " + snip
	}
	return "[history] — " + snip
}

// VectorExcerpt clamps a non-history vector snippet.
func VectorExcerpt(vectorSnippet string) string {
	return ClampExcerpt(vectorSnippet, 200, 2)
}

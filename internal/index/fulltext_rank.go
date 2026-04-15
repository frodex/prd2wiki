package index

import (
	"strings"
	"unicode"
)

// MatchTier classifies metadata match strength for title+tags vs query.
// Lower sorts first: 0 = phrase in blob; 1 = every query token in title; 2 = every token in title+tags but not all in title alone; 3 = weaker.
func MatchTier(title, tags, query string) int {
	tit := normalizeForTitleMatch(title)
	blob := matchMetadataBlob(title, tags)
	q := normalizeForTitleMatch(query)
	if blob == "" || q == "" {
		return 3
	}
	if strings.Contains(blob, q) {
		return 0
	}
	tokens := significantQueryTokens(q)
	if len(tokens) == 0 {
		return 3
	}
	allInTitle := true
	for _, tok := range tokens {
		if !strings.Contains(tit, tok) {
			allInTitle = false
			break
		}
	}
	if allInTitle {
		return 1
	}
	for _, tok := range tokens {
		if !strings.Contains(blob, tok) {
			return 3
		}
	}
	return 2
}

func significantQueryTokens(q string) []string {
	var out []string
	for _, tok := range strings.Fields(q) {
		if len([]rune(tok)) >= 2 {
			out = append(out, tok)
		}
	}
	return out
}

func matchMetadataBlob(title, tags string) string {
	t := normalizeForTitleMatch(title)
	tags = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(tags), ",", " "))
	tags = strings.Join(strings.Fields(tags), " ")
	switch {
	case t == "":
		return tags
	case tags == "":
		return t
	default:
		return t + " " + tags
	}
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
		if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

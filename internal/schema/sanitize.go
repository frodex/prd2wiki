package schema

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// SanitizePathSegment strips dangerous characters from a path segment to prevent
// path traversal, injection, null bytes, and shell metacharacters.
// Only allows: lowercase alphanumeric, hyphens, underscores.
// Spaces become hyphens. Unicode diacritics are stripped via NFKD decomposition
// so "résumé" → "resume", "naïve" → "naive", "über" → "uber".
func SanitizePathSegment(s string) string {
	// NFKD decompose then strip combining marks (diacritics)
	decomposed := norm.NFKD.String(s)
	var buf strings.Builder
	for _, r := range strings.ToLower(decomposed) {
		if unicode.Is(unicode.Mn, r) {
			continue // skip combining marks (diacritics)
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			buf.WriteRune(r)
		} else if r == ' ' {
			buf.WriteRune('-')
		}
	}
	result := buf.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	result = strings.Trim(result, "-")
	if result == "" {
		return "untitled"
	}
	return result
}

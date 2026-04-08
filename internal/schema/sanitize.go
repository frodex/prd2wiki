package schema

import "strings"

// SanitizePathSegment strips dangerous characters from a path segment to prevent
// path traversal, injection, null bytes, and shell metacharacters.
// Only allows: lowercase alphanumeric, hyphens, underscores.
// Spaces become hyphens. Everything else is dropped.
func SanitizePathSegment(s string) string {
	var buf strings.Builder
	for _, r := range strings.ToLower(s) {
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

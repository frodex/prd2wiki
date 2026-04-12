// Package pagepath resolves wiki page IDs to git storage paths. Used by API and web.
package pagepath

import (
	"fmt"

	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/schema"
)

// Resolve returns the preferred storage path for a page ID: SQLite index first,
// then inferred hash-prefix or flat layout.
func Resolve(search *index.Searcher, project, id string) string {
	results, err := search.ByID(project, id)
	if err == nil && len(results) > 0 && results[0].Path != "" {
		return results[0].Path
	}
	sanitized := schema.SanitizePathSegment(id)
	if schema.IsHashID(sanitized) && len(sanitized) >= 2 {
		return fmt.Sprintf("pages/%s/%s.md", sanitized[:2], sanitized[2:])
	}
	return fmt.Sprintf("pages/%s.md", sanitized)
}

// Alternate returns the other path form (hash-prefix vs flat) when both are meaningful, else "".
func Alternate(id, currentPath string) string {
	sanitized := schema.SanitizePathSegment(id)
	flat := fmt.Sprintf("pages/%s.md", sanitized)
	if len(sanitized) < 2 {
		return ""
	}
	hashPrefix := fmt.Sprintf("pages/%s/%s.md", sanitized[:2], sanitized[2:])
	if currentPath == hashPrefix {
		return flat
	}
	return hashPrefix
}

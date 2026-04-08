package path

import (
	"github.com/frodex/prd2wiki/internal/schema"
)

// ResolvePath determines the git path for a page by its ID.
// Tries: index lookup -> hash-prefix -> flat legacy.
// Returns the path and whether it was found.
func ResolvePath(id string) string {
	// Hash IDs: pages/{first2}/{rest}.md
	if schema.IsHashID(id) {
		return "pages/" + id[:2] + "/" + id[2:] + ".md"
	}
	// Legacy: pages/{id}.md
	return "pages/" + id + ".md"
}

// AlternatePath returns the other possible path format for a page.
func AlternatePath(id, currentPath string) string {
	hashPath := "pages/" + id[:2] + "/" + id[2:] + ".md"
	flatPath := "pages/" + id + ".md"
	if currentPath == hashPath {
		return flatPath
	}
	return hashPath
}

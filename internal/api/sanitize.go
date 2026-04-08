package api

import "github.com/frodex/prd2wiki/internal/schema"

// sanitizePageID delegates to the shared sanitizer.
func sanitizePageID(id string) string {
	return schema.SanitizePathSegment(id)
}

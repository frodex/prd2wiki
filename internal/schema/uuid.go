package schema

import (
	"strings"

	"github.com/google/uuid"
)

// IsUUIDPageID reports whether id is a canonical UUID string (used for tree pages at pages/{uuid}.md).
func IsUUIDPageID(id string) bool {
	if id == "" {
		return false
	}
	_, err := uuid.Parse(strings.TrimSpace(id))
	return err == nil
}

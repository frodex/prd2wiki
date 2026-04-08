package schema

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// GeneratePageID creates a short content-addressed hash ID for a new page.
// Input: title + creation timestamp. Output: 7-char hex string.
// This ID is fixed at creation and never changes even if the content changes.
func GeneratePageID(title string, created time.Time) string {
	input := fmt.Sprintf("%s|%d", title, created.UnixNano())
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash[:4])[:7] // 7 hex chars
}

// IsHashID returns true if the given ID looks like an auto-generated hash ID
// (exactly 7 lowercase hex characters). This is used to decide whether to
// store a page in a hash-prefix directory or a flat directory.
func IsHashID(id string) bool {
	if len(id) != 7 {
		return false
	}
	for _, r := range id {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

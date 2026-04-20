package tree

import (
	"fmt"
	"regexp"
	"strings"
)

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

// SlugFromTitle produces a URL slug: lowercase, hyphens, alphanumeric only.
func SlugFromTitle(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = nonSlugChars.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "untitled"
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

// UniqueSlug returns base or base-2, base-3, ... until unused.
func UniqueSlug(base string, used map[string]bool) string {
	s := base
	n := 2
	for used[s] {
		s = fmt.Sprintf("%s-%d", base, n)
		n++
	}
	used[s] = true
	return s
}

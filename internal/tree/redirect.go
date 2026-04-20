package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteLeafRedirect leaves a redirect at oldURLPath after a .link was moved or renamed away.
// oldURLPath is a web path with forward slashes, no leading slash (e.g. "prd2wiki/old-slug").
// location is the new URL path (leading /) or an absolute http(s) URL.
// permanent selects HTTP 301 (.301) vs 302 (.302) on disk.
func WriteLeafRedirect(treeRoot, oldURLPath, location string, permanent bool) error {
	treeRoot = filepath.Clean(treeRoot)
	oldURLPath = strings.Trim(oldURLPath, "/")
	location = strings.TrimSpace(location)
	if oldURLPath == "" || location == "" {
		return fmt.Errorf("tree.WriteLeafRedirect: empty path or location")
	}
	if err := validateTreeURLPath(oldURLPath); err != nil {
		return fmt.Errorf("tree.WriteLeafRedirect: %w", err)
	}
	name := ".302"
	if permanent {
		name = ".301"
	}
	dir := filepath.Join(treeRoot, filepath.FromSlash(oldURLPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("tree.WriteLeafRedirect: mkdir: %w", err)
	}
	p := filepath.Join(dir, name)
	content := location
	if !strings.HasPrefix(location, "http://") && !strings.HasPrefix(location, "https://") && !strings.HasPrefix(location, "/") {
		content = "/" + strings.TrimPrefix(location, "/")
	}
	return os.WriteFile(p, []byte(strings.TrimSpace(content)+"\n"), 0o644)
}

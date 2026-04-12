package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteLinkFile writes {slug}.link under treeRoot/projectRel (e.g. tree/prd2wiki).
func WriteLinkFile(treeRoot, projectRel, slug, pageUUID, title string) error {
	dir := filepath.Join(treeRoot, filepath.FromSlash(projectRel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := slug + ".link"
	if !safeSlug(slug) {
		return fmt.Errorf("invalid slug %q", slug)
	}
	p := filepath.Join(dir, name)
	content := strings.TrimSpace(pageUUID) + "\n\n" + strings.TrimSpace(title) + "\n"
	return os.WriteFile(p, []byte(content), 0o644)
}

// DeleteLinkFile removes {slug}.link from the project tree directory.
func DeleteLinkFile(treeRoot, projectRel, slug string) error {
	if !safeSlug(slug) {
		return fmt.Errorf("invalid slug %q", slug)
	}
	p := filepath.Join(treeRoot, filepath.FromSlash(projectRel), slug+".link")
	if err := os.Remove(p); err != nil {
		return err
	}
	return nil
}

func safeSlug(s string) bool {
	if s == "" || strings.Contains(s, "..") || strings.Contains(s, "/") || strings.Contains(s, "\\") {
		return false
	}
	return true
}

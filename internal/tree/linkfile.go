package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteProjectUUIDFile writes .uuid under treeRoot/projectRel (e.g. tree/prd2wiki).
func WriteProjectUUIDFile(treeRoot, projectRel, projectUUID, displayName string) error {
	dir := filepath.Join(treeRoot, filepath.FromSlash(projectRel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p := filepath.Join(dir, ".uuid")
	content := strings.TrimSpace(projectUUID) + "\n" + strings.TrimSpace(displayName) + "\n"
	return os.WriteFile(p, []byte(content), 0o644)
}

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

// WriteLinkFileAtTreeURL writes treeRoot/{urlPath}.link, creating parent directories.
// urlPath uses web slashes (e.g. "prd2wiki/plan-foo"). Each path segment must pass safeSlug.
func WriteLinkFileAtTreeURL(treeRoot, urlPath, pageUUID, title string) error {
	urlPath = strings.Trim(urlPath, "/")
	if urlPath == "" {
		return fmt.Errorf("empty tree url path")
	}
	if strings.Contains(urlPath, "..") {
		return fmt.Errorf("invalid tree url path")
	}
	for _, seg := range strings.Split(urlPath, "/") {
		if seg == "" {
			return fmt.Errorf("invalid empty segment in tree url path")
		}
		if !safeSlug(seg) {
			return fmt.Errorf("invalid segment %q in tree url path", seg)
		}
	}
	rel := filepath.FromSlash(urlPath) + ".link"
	p := filepath.Join(treeRoot, rel)
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
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

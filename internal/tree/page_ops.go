package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MovePage moves a page’s .link file so its canonical tree URL changes.
// oldURLPath and newURLPath are web-style paths: forward slashes, no leading slash,
// no ".link" suffix (e.g. "prd2wiki/my-page"). Parent directories are created as needed.
// Callers should refresh tree.IndexHolder after a successful move.
func MovePage(treeRoot, oldURLPath, newURLPath string) error {
	treeRoot = filepath.Clean(treeRoot)
	oldURLPath = strings.Trim(oldURLPath, "/")
	newURLPath = strings.Trim(newURLPath, "/")
	if oldURLPath == "" || newURLPath == "" {
		return fmt.Errorf("tree.MovePage: empty path")
	}
	if oldURLPath == newURLPath {
		return nil
	}
	if err := validateTreeURLPath(oldURLPath); err != nil {
		return fmt.Errorf("tree.MovePage old: %w", err)
	}
	if err := validateTreeURLPath(newURLPath); err != nil {
		return fmt.Errorf("tree.MovePage new: %w", err)
	}
	oldFile := linkFileAbs(treeRoot, oldURLPath)
	newFile := linkFileAbs(treeRoot, newURLPath)
	if st, err := os.Stat(oldFile); err != nil || st.IsDir() {
		if err != nil {
			return fmt.Errorf("tree.MovePage: old link %q: %w", oldFile, err)
		}
		return fmt.Errorf("tree.MovePage: old path is not a file: %s", oldFile)
	}
	if _, err := os.Stat(newFile); err == nil {
		return fmt.Errorf("tree.MovePage: destination already exists: %s", newFile)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("tree.MovePage: stat destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(newFile), 0o755); err != nil {
		return fmt.Errorf("tree.MovePage: mkdir: %w", err)
	}
	if err := os.Rename(oldFile, newFile); err != nil {
		return fmt.Errorf("tree.MovePage: rename: %w", err)
	}
	return nil
}

// RenamePage changes the slug (filename) for a page within one project tree directory.
// projectRel is the project path under the tree root (e.g. "prd2wiki" or "games/battletech").
// Callers should refresh tree.IndexHolder after a successful rename.
func RenamePage(treeRoot, projectRel, oldSlug, newSlug string) error {
	projectRel = strings.Trim(projectRel, "/")
	if !safeSlug(oldSlug) {
		return fmt.Errorf("tree.RenamePage: invalid old slug %q", oldSlug)
	}
	if !safeSlug(newSlug) {
		return fmt.Errorf("tree.RenamePage: invalid new slug %q", newSlug)
	}
	if oldSlug == newSlug {
		return nil
	}
	return MovePage(treeRoot, projectRel+"/"+oldSlug, projectRel+"/"+newSlug)
}

func linkFileAbs(treeRoot, urlPath string) string {
	return filepath.Join(treeRoot, filepath.FromSlash(urlPath)+".link")
}

func validateTreeURLPath(p string) error {
	if strings.Contains(p, "..") {
		return fmt.Errorf("invalid path %q", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" {
			return fmt.Errorf("invalid empty segment in %q", p)
		}
		if seg == "." || seg == ".." {
			return fmt.Errorf("invalid segment %q in %q", seg, p)
		}
	}
	return nil
}

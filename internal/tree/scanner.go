package tree

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Index is the result of scanning tree/ — projects, pages, and URL lookups.
type Index struct {
	Projects   []*Project
	byURLPath  map[string]*PageEntry
	byPageUUID map[string]*PageEntry
}

// PageEntry ties a page to its project for routing and redirects.
type PageEntry struct {
	Project *Project
	Page    *Page
}

// URLPath returns the web path (no leading slash) for this page, e.g. "prd2wiki/my-slug".
func (e *PageEntry) URLPath() string {
	if e == nil || e.Page == nil {
		return ""
	}
	return e.Page.TreePath
}

// Scan walks treeRoot (absolute) and discovers projects and pages. dataDir (absolute)
// is used to resolve data/repos/proj_*.git and to map .wiki.git symlink names to RepoKey.
func Scan(treeRoot, dataDir string) (*Index, error) {
	treeRoot = filepath.Clean(treeRoot)
	dataDir = filepath.Clean(dataDir)
	if treeRoot == "" || dataDir == "" {
		return nil, fmt.Errorf("tree.Scan: empty path")
	}
	if fi, err := os.Lstat(treeRoot); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("tree root %q must not be a symbolic link", treeRoot)
	}

	uuid8ToKey, err := discoverRepoKeys(dataDir)
	if err != nil {
		return nil, err
	}

	idx := &Index{
		byURLPath:  make(map[string]*PageEntry),
		byPageUUID: make(map[string]*PageEntry),
	}

	// Phase 1: find every directory that contains .uuid
	var projects []*Project
	err = filepath.WalkDir(treeRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != ".uuid" {
			return nil
		}
		dir := filepath.Dir(path)
		rel, err := filepath.Rel(treeRoot, dir)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.Contains(rel, "..") {
			return nil
		}

		uuid, name, err := readUUIDFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		pre := strings.SplitN(uuid, "-", 2)[0]
		repoKey, ok := uuid8ToKey[pre]
		if !ok {
			var inferOK bool
			repoKey, inferOK = inferRepoKey(dataDir, uuid)
			if !inferOK {
				return fmt.Errorf("tree project %q: could not map UUID prefix %q to a wiki repo under %q (need data/*.wiki.git → repos/proj_%s.git or equivalent)", rel, pre, dataDir, pre)
			}
		}
		rp := RepoPathForUUID(dataDir, uuid)
		if rp == "" {
			return fmt.Errorf("invalid project uuid in %s", path)
		}
		p := &Project{
			UUID:     uuid,
			Name:     name,
			Path:     rel,
			RepoPath: rp,
			RepoKey:  repoKey,
		}
		projects = append(projects, p)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(projects, func(i, j int) bool {
		return len(projects[i].Path) > len(projects[j].Path)
	})
	idx.Projects = projects

	// Phase 2: *.link files
	err = filepath.WalkDir(treeRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".link") {
			return nil
		}
		relFull, err := filepath.Rel(treeRoot, path)
		if err != nil {
			return err
		}
		relFull = filepath.ToSlash(relFull)
		if strings.Contains(relFull, "..") {
			return nil
		}

		dir := filepath.ToSlash(filepath.Dir(relFull))
		proj := resolveProject(dir, projects)
		if proj == nil {
			return fmt.Errorf(".link outside any project: %s", relFull)
		}

		slug := strings.TrimSuffix(filepath.Base(relFull), ".link")
		page, err := parseLinkFile(path, slug, relFull)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}

		ent := &PageEntry{Project: proj, Page: page}
		urlKey := page.TreePath
		idx.byURLPath[urlKey] = ent
		if _, dup := idx.byPageUUID[page.UUID]; dup {
			return fmt.Errorf("duplicate page UUID %s", page.UUID)
		}
		idx.byPageUUID[page.UUID] = ent
		return nil
	})
	if err != nil {
		return nil, err
	}

	return idx, nil
}

func readUUIDFile(path string) (uuid, display string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) < 2 {
		return "", "", fmt.Errorf(".uuid: want 2 lines, got %d", len(lines))
	}
	return strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1]), nil
}

func parseLinkFile(path, slug, relFull string) (*Page, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) < 1 || strings.TrimSpace(lines[0]) == "" {
		return nil, fmt.Errorf(".link: missing page UUID on line 1")
	}
	pageUUID := strings.TrimSpace(lines[0])
	lib := ""
	if len(lines) >= 2 {
		lib = strings.TrimSpace(lines[1])
	}
	title := ""
	if len(lines) >= 3 {
		title = strings.TrimSpace(lines[2])
		if len(lines) > 3 {
			rest := strings.TrimSpace(strings.Join(lines[3:], "\n"))
			if rest != "" {
				if title != "" {
					title = title + "\n" + rest
				} else {
					title = rest
				}
			}
		}
	}

	treePath := strings.TrimSuffix(relFull, ".link")
	treePath = filepath.ToSlash(treePath)

	return &Page{
		UUID:        pageUUID,
		LibrarianID: lib,
		Title:       title,
		Slug:        slug,
		TreePath:    treePath,
	}, nil
}

func resolveProject(dir string, projectsSortedLongestFirst []*Project) *Project {
	for _, p := range projectsSortedLongestFirst {
		if dir == p.Path || strings.HasPrefix(dir, p.Path+"/") {
			return p
		}
	}
	return nil
}

// discoverRepoKeys maps the first UUID segment (before "-") to wiki repo key (e.g. "default")
// by reading symlinks data/{key}.wiki.git → repos/proj_{segment}.git
// inferRepoKey matches a project UUID to a wiki repo key by finding data/{key}.wiki.git
// that shares the same file as data/repos/proj_{prefix}.git (follows symlinks via os.SameFile).
func inferRepoKey(dataDir, uuid string) (string, bool) {
	rp := RepoPathForUUID(dataDir, uuid)
	stR, err := os.Stat(rp)
	if err != nil {
		return "", false
	}
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".wiki.git") {
			continue
		}
		full := filepath.Join(dataDir, e.Name())
		st, err := os.Stat(full)
		if err != nil {
			continue
		}
		if os.SameFile(stR, st) {
			return strings.TrimSuffix(e.Name(), ".wiki.git"), true
		}
	}
	return "", false
}

func discoverRepoKeys(dataDir string) (map[string]string, error) {
	m := make(map[string]string)
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".wiki.git") {
			continue
		}
		key := strings.TrimSuffix(name, ".wiki.git")
		full := filepath.Join(dataDir, name)
		fi, err := os.Lstat(full)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			// Non-symlink bare repo at data/name.wiki.git — cannot infer proj_ id; skip.
			continue
		}
		targ, err := os.Readlink(full)
		if err != nil {
			continue
		}
		targ = filepath.Clean(targ)
		base := filepath.Base(targ)
		if !strings.HasPrefix(base, "proj_") || !strings.HasSuffix(base, ".git") {
			continue
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(base, "proj_"), ".git")
		m[inner] = key
	}
	return m, nil
}

// PageByURLPath returns the page entry for a normalized tree URL path (no leading/trailing slash), e.g. "prd2wiki/foo".
func (x *Index) PageByURLPath(urlPath string) (*PageEntry, bool) {
	if x == nil {
		return nil, false
	}
	urlPath = strings.Trim(urlPath, "/")
	e, ok := x.byURLPath[urlPath]
	return e, ok
}

// PageByUUID returns the page entry for a page UUID (from git frontmatter / .link).
func (x *Index) PageByUUID(pageUUID string) (*PageEntry, bool) {
	if x == nil {
		return nil, false
	}
	e, ok := x.byPageUUID[strings.TrimSpace(pageUUID)]
	return e, ok
}

package web

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/frodex/prd2wiki/internal/tree"
)

// WithTreeRouter wraps inner so GET requests for non-reserved paths are served from the
// on-disk tree (.link → git page) when the tree index is available.
func (h *Handler) WithTreeRouter(treeRootAbs string, holder *tree.IndexHolder, inner http.Handler) http.Handler {
	if holder == nil {
		return inner
	}
	root := filepath.Clean(treeRootAbs)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			inner.ServeHTTP(w, r)
			return
		}
		p := r.URL.Path
		if p == "/" || tree.IsReservedRequestPath(p) {
			inner.ServeHTTP(w, r)
			return
		}
		idx := holder.Get()
		if idx == nil {
			inner.ServeHTTP(w, r)
			return
		}
		if handled := h.serveTreePage(w, r, root, idx); handled {
			return
		}
		inner.ServeHTTP(w, r)
	})
}

func (h *Handler) serveTreePage(w http.ResponseWriter, r *http.Request, treeRoot string, idx *tree.Index) bool {
	parts, err := splitValidatedPathSegments(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return true
	}
	if len(parts) == 0 {
		return false
	}

	urlPath := strings.Join(parts, "/")
	if proj, ok := idx.ProjectByTreePath(urlPath); ok {
		h.serveTreeDirectory(w, r, proj, idx, urlPath)
		return true
	}

	cur := treeRoot
	for i := 0; i < len(parts)-1; i++ {
		next := filepath.Join(cur, parts[i])
		fi, err := os.Lstat(next)
		if err != nil || !fi.IsDir() {
			http.NotFound(w, r)
			return true
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			http.Error(w, "invalid path", http.StatusForbidden)
			return true
		}
		cur = next
		if code, loc, ok := readTreeRedirect(cur); ok {
			rem := strings.Join(parts[i+1:], "/")
			http.Redirect(w, r, joinRedirectLocation(r, loc, rem), code)
			return true
		}
	}

	linkPath := filepath.Join(cur, parts[len(parts)-1]+".link")
	fi, err := os.Lstat(linkPath)
	if err != nil || fi.IsDir() {
		// After a move/rename, the old .link may be replaced by a stub directory with .301/.302.
		leafDir := filepath.Join(cur, parts[len(parts)-1])
		if fi2, err2 := os.Lstat(leafDir); err2 == nil && fi2.IsDir() {
			if code, loc, ok := readTreeRedirect(leafDir); ok {
				http.Redirect(w, r, joinRedirectLocation(r, loc, ""), code)
				return true
			}
		}
		http.NotFound(w, r)
		return true
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		http.Error(w, "invalid path", http.StatusForbidden)
		return true
	}

	urlKey := strings.Join(parts, "/")
	ent, ok := idx.PageByURLPath(urlKey)
	if !ok {
		http.NotFound(w, r)
		return true
	}

	repo, ok := h.repos[ent.Project.RepoKey]
	if !ok {
		http.Error(w, "project unavailable", http.StatusInternalServerError)
		return true
	}

	gitPath := "pages/" + strings.TrimSpace(ent.Page.UUID) + ".md"
	h.viewPageAtGitPath(w, ent.Project.RepoKey, gitPath, repo)
	return true
}

func (h *Handler) serveTreeDirectory(w http.ResponseWriter, r *http.Request, proj *tree.Project, idx *tree.Index, urlPath string) {
	var entries []TreeDirEntry
	for _, e := range idx.AllPageEntries() {
		if e.Project.Path != proj.Path {
			continue
		}
		title := strings.TrimSpace(e.Page.Title)
		if title == "" {
			title = e.Page.Slug
		}
		entries = append(entries, TreeDirEntry{
			Title: title,
			Slug:  e.Page.Slug,
			Href:  "/" + e.Page.TreePath,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Title) < strings.ToLower(entries[j].Title)
	})
	data := PageData{
		Project:     proj.RepoKey,
		Title:       proj.Name + " — wiki",
		Content:     TreeDirectoryData{ProjectName: proj.Name, TreePath: urlPath, Pages: entries},
		Breadcrumbs: treeBreadcrumbs(idx, urlPath, ""),
	}
	h.preparePageData(&data)
	t := h.templates["templates/tree_directory.html"]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

func splitValidatedPathSegments(raw string) ([]string, error) {
	dec, err := url.PathUnescape(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	p := path.Clean("/" + dec)
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil, nil
	}
	parts := strings.Split(p, "/")
	for _, s := range parts {
		if s == "" || s == "." || s == ".." {
			return nil, errInvalidTreePath
		}
	}
	return parts, nil
}

var errInvalidTreePath = errors.New("invalid path")

func readTreeRedirect(dir string) (code int, location string, ok bool) {
	for _, rd := range []struct {
		name string
		code int
	}{
		{".301", http.StatusMovedPermanently},
		{".302", http.StatusFound},
	} {
		p := filepath.Join(dir, rd.name)
		fi, err := os.Lstat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		line := strings.TrimSpace(strings.Split(string(data), "\n")[0])
		if line == "" {
			continue
		}
		return rd.code, line, true
	}
	return 0, "", false
}

func joinRedirectLocation(r *http.Request, loc, remainder string) string {
	loc = strings.TrimSpace(loc)
	if strings.HasPrefix(loc, "http://") || strings.HasPrefix(loc, "https://") {
		u, err := url.Parse(loc)
		if err != nil {
			return loc
		}
		u.Path = path.Clean("/" + u.Path + "/" + remainder)
		if r.URL.RawQuery != "" {
			u.RawQuery = r.URL.RawQuery
		}
		return u.String()
	}
	if !strings.HasPrefix(loc, "/") {
		loc = "/" + loc
	}
	out := path.Clean(loc + "/" + remainder)
	if r.URL.RawQuery != "" {
		return out + "?" + r.URL.RawQuery
	}
	return out
}

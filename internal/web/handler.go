package web

import (
	"database/sql"
	"embed"
	"html/template"
	"net/http"
	"sort"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/pagepath"
	"github.com/frodex/prd2wiki/internal/tree"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

//go:embed templates/*.html static/* static/css/*
var content embed.FS

// md is the goldmark instance with GFM extensions (tables, strikethrough, autolinks, task lists).
var md = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
	),
)

// PageData is the top-level data passed to every template.
type PageData struct {
	Project  string
	Title    string
	Content  interface{} // varies per template
	Projects []string    // for nav
}

// Handler serves the wiki web UI.
type Handler struct {
	repos      map[string]*wgit.Repo
	search     *index.Searcher
	librarians map[string]*librarian.Librarian
	db         *sql.DB
	templates  map[string]*template.Template
	edits      map[string]*EditCache // per-project edit info cache
	treeIdx    *tree.Index          // optional; tree URLs and legacy redirects
}

// NewHandler creates a Handler with pre-parsed templates.
func NewHandler(repos map[string]*wgit.Repo, db *sql.DB, librarians map[string]*librarian.Librarian, treeIdx *tree.Index) *Handler {
	h := &Handler{
		repos:      repos,
		search:     index.NewSearcher(db),
		librarians: librarians,
		db:         db,
		templates:  make(map[string]*template.Template),
		edits:      make(map[string]*EditCache),
		treeIdx:    treeIdx,
	}

	// Build edit caches in background so startup isn't blocked.
	for project, repo := range repos {
		cache := NewEditCache()
		h.edits[project] = cache
		go func(proj string, r *wgit.Repo, c *EditCache) {
			searcher := index.NewSearcher(db)
			pages, err := searcher.ListAll(proj)
			if err != nil {
				return
			}
			paths := make([]string, len(pages))
			for i, p := range pages {
				paths[i] = p.Path
			}
			c.Build(r, paths)
		}(project, repo, cache)
	}

	// Parse each page template together with the layout.
	pageTemplates := []string{
		"templates/home.html",
		"templates/page_list.html",
		"templates/page_view.html",
		"templates/page_edit.html",
		"templates/search.html",
		"templates/page_history.html",
		"templates/page_diff.html",
		"templates/error.html",
	}
	for _, pt := range pageTemplates {
		// page_view needs the page_actions partial
		if pt == "templates/page_view.html" {
			t := template.Must(template.ParseFS(content, "templates/layout.html", "templates/page_actions.html", pt))
			h.templates[pt] = t
		} else {
			t := template.Must(template.ParseFS(content, "templates/layout.html", pt))
			h.templates[pt] = t
		}
	}

	return h
}

// EditCaches returns the per-project edit caches so the API server can call Touch on writes.
func (h *Handler) EditCaches() map[string]*EditCache {
	return h.edits
}

// Register adds web routes to the given ServeMux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.home)
	mux.HandleFunc("GET /projects/{project}/pages", h.listPages)
	mux.HandleFunc("GET /projects/{project}/pages/{id}", h.viewPage)
	mux.HandleFunc("GET /projects/{project}/pages/{id}/edit", h.editPage)
	mux.HandleFunc("GET /projects/{project}/pages/{id}/history", h.pageHistory)
	mux.HandleFunc("GET /projects/{project}/pages/{id}/history/{hash}", h.pageAtCommitView)
	mux.HandleFunc("GET /projects/{project}/pages/{id}/diff", h.pageDiff)
	mux.HandleFunc("GET /projects/{project}/pages/new", h.newPage)
	mux.HandleFunc("GET /projects/{project}/search", h.searchPages)
	mux.Handle("GET /static/", http.FileServerFS(content))
}

// projects returns the list of configured project names.
func (h *Handler) projects() []string {
	names := make([]string, 0, len(h.repos))
	for name := range h.repos {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// resolvePagePath looks up the stored path for a page ID from the SQLite index.
// Falls back to hash-prefix path for hash IDs, or flat "pages/{id}.md" for legacy IDs.
func (h *Handler) resolvePagePath(project, id string) string {
	return pagepath.Resolve(h.search, project, id)
}

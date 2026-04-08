package web

import (
	"database/sql"
	"embed"
	"html/template"
	"net/http"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
)

//go:embed templates/*.html static/*
var content embed.FS

// PageData is the top-level data passed to every template.
type PageData struct {
	Project  string
	Title    string
	Content  interface{} // varies per template
	Projects []string    // for nav
}

// PageListItem represents one row in the page listing table.
type PageListItem struct {
	ID         string
	Title      string
	Type       string
	Status     string
	TrustLevel int
	Path       string
}

// Handler serves the wiki web UI.
type Handler struct {
	repos      map[string]*wgit.Repo
	search     *index.Searcher
	librarians map[string]*librarian.Librarian
	db         *sql.DB
	templates  map[string]*template.Template
}

// NewHandler creates a Handler with pre-parsed templates.
func NewHandler(repos map[string]*wgit.Repo, db *sql.DB, librarians map[string]*librarian.Librarian) *Handler {
	h := &Handler{
		repos:      repos,
		search:     index.NewSearcher(db),
		librarians: librarians,
		db:         db,
		templates:  make(map[string]*template.Template),
	}

	// Parse each page template together with the layout.
	pageTemplates := []string{
		"templates/page_list.html",
	}
	for _, pt := range pageTemplates {
		t := template.Must(template.ParseFS(content, "templates/layout.html", pt))
		h.templates[pt] = t
	}

	return h
}

// Register adds web routes to the given ServeMux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.home)
	mux.HandleFunc("GET /projects/{project}/pages", h.listPages)
	mux.HandleFunc("GET /projects/{project}/pages/{id}", h.viewPage)
	mux.HandleFunc("GET /projects/{project}/pages/{id}/edit", h.editPage)
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
	return names
}

// home redirects to the default project's page list.
func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/projects/default/pages", http.StatusFound)
}

// listPages renders the page listing for a project.
func (h *Handler) listPages(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")

	results, err := h.search.ListAll(project)
	if err != nil {
		http.Error(w, "failed to list pages: "+err.Error(), http.StatusInternalServerError)
		return
	}

	items := make([]PageListItem, len(results))
	for i, pr := range results {
		items[i] = PageListItem{
			ID:         pr.ID,
			Title:      pr.Title,
			Type:       pr.Type,
			Status:     pr.Status,
			TrustLevel: pr.TrustLevel,
			Path:       pr.Path,
		}
	}

	data := PageData{
		Project:  project,
		Title:    project + " — Pages",
		Content:  items,
		Projects: h.projects(),
	}

	t := h.templates["templates/page_list.html"]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// viewPage renders a single wiki page (stub for future implementation).
func (h *Handler) viewPage(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// editPage renders the page edit form (stub for future implementation).
func (h *Handler) editPage(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// newPage renders the new page form (stub for future implementation).
func (h *Handler) newPage(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// searchPages renders search results (stub for future implementation).
func (h *Handler) searchPages(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

package web

import (
	"bytes"
	"database/sql"
	"embed"
	"html/template"
	"net/http"
	"strings"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/schema"
	"github.com/yuin/goldmark"
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

// PageViewData holds data for the page view template.
type PageViewData struct {
	ID         string
	Title      string
	Type       string
	Status     string
	TrustLevel int
	Creator    string
	Created    string
	Modified   string
	Tags       []string
	BodyHTML   template.HTML
	Sources    []schema.Source
}

// PageEditData holds data for the page edit template.
type PageEditData struct {
	IsNew   bool
	ID      string
	Title   string
	Type    string
	Status  string
	TagsCSV string
	Body    string
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
		"templates/page_view.html",
		"templates/page_edit.html",
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

// viewPage renders a single wiki page.
func (h *Handler) viewPage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")

	repo, ok := h.repos[project]
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// Determine the page path from the id.
	path := "pages/" + id + ".md"

	// Try truth branch first, fall back to draft/incoming.
	fm, body, err := repo.ReadPageWithMeta("truth", path)
	if err != nil {
		fm, body, err = repo.ReadPageWithMeta("draft/incoming", path)
		if err != nil {
			http.Error(w, "page not found: "+err.Error(), http.StatusNotFound)
			return
		}
	}

	// Render markdown body to HTML.
	var htmlBuf bytes.Buffer
	if err := goldmark.Convert(body, &htmlBuf); err != nil {
		http.Error(w, "markdown render error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	pvd := PageViewData{
		ID:         fm.ID,
		Title:      fm.Title,
		Type:       fm.Type,
		Status:     fm.Status,
		TrustLevel: fm.TrustLevel,
		Creator:    fm.DCCreator,
		Tags:       fm.Tags,
		BodyHTML:   template.HTML(htmlBuf.String()),
		Sources:    fm.Provenance.Sources,
	}
	if !fm.DCCreated.IsZero() {
		pvd.Created = fm.DCCreated.Format("2006-01-02")
	}
	if !fm.DCModified.IsZero() {
		pvd.Modified = fm.DCModified.Format("2006-01-02")
	}

	data := PageData{
		Project:  project,
		Title:    fm.Title + " — " + project,
		Content:  pvd,
		Projects: h.projects(),
	}

	t := h.templates["templates/page_view.html"]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// editPage renders the page edit form with existing page content.
func (h *Handler) editPage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")

	repo, ok := h.repos[project]
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	path := "pages/" + id + ".md"

	fm, body, err := repo.ReadPageWithMeta("truth", path)
	if err != nil {
		fm, body, err = repo.ReadPageWithMeta("draft/incoming", path)
		if err != nil {
			http.Error(w, "page not found: "+err.Error(), http.StatusNotFound)
			return
		}
	}

	ped := PageEditData{
		IsNew:   false,
		ID:      fm.ID,
		Title:   fm.Title,
		Type:    fm.Type,
		Status:  fm.Status,
		TagsCSV: strings.Join(fm.Tags, ", "),
		Body:    string(body),
	}

	data := PageData{
		Project:  project,
		Title:    "Edit: " + fm.Title,
		Content:  ped,
		Projects: h.projects(),
	}

	t := h.templates["templates/page_edit.html"]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// newPage renders the new page form with default values.
func (h *Handler) newPage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")

	if _, ok := h.repos[project]; !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	ped := PageEditData{
		IsNew:  true,
		Type:   "concept",
		Status: "draft",
	}

	data := PageData{
		Project:  project,
		Title:    "New Page",
		Content:  ped,
		Projects: h.projects(),
	}

	t := h.templates["templates/page_edit.html"]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// searchPages renders search results (stub for future implementation).
func (h *Handler) searchPages(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

package web

import (
	"bytes"
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

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
	ID           string
	Title        string
	Type         string
	Status       string
	TrustLevel   int
	Path         string
	LastEditBy   string
	LastEditDate string
	Score        string // similarity score for search results
}

// PageViewData holds data for the page view template.
type PageViewData struct {
	Project      string
	ID           string
	Title        string
	Type         string
	Status       string
	TrustLevel   int
	Creator      string
	Created      string
	Modified     string
	Tags         []string
	BodyHTML      template.HTML
	Sources      []schema.Source
	Branch       string
	LastEditBy   string
	LastEditDate string
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

// SearchData holds data for the search results template.
type SearchData struct {
	Query   string
	Type    string
	Status  string
	Tag     string
	Results []PageListItem
}

// HistoryCommit is a single commit in the history view.
type HistoryCommit struct {
	Hash      string
	ShortHash string
	Author    string
	DateStr   string
	Message   string
}

// PageHistoryData holds data for the page history template.
type PageHistoryData struct {
	ID      string
	Title   string
	Commits []HistoryCommit
}

// DiffChange represents one line in a diff.
type DiffChange struct {
	Type    string // "context", "add", "delete"
	Content string
}

// PageDiffData holds data for the page diff template.
type PageDiffData struct {
	ID        string
	Title     string
	From      string
	To        string
	FromShort string
	ToShort   string
	Changes   []DiffChange
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
		"templates/search.html",
		"templates/page_history.html",
		"templates/page_diff.html",
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

	repo := h.repos[project]

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
		if repo != nil {
			commits, _ := repo.PageHistoryAllBranches(pr.Path, 1)
			if len(commits) > 0 {
				items[i].LastEditBy = commits[0].Author
				items[i].LastEditDate = commits[0].Date.Format("2006-01-02 15:04")
			}
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].LastEditDate > items[j].LastEditDate
	})

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

// readPageNewest finds the most recently modified version of a page across all branches.
// Returns the frontmatter, body, and the branch it was found on.
func readPageNewest(repo *wgit.Repo, path string) (*schema.Frontmatter, []byte, string, error) {
	branches, err := repo.ListBranches()
	if err != nil || len(branches) == 0 {
		branches = []string{"truth", "draft/incoming"}
	}

	type candidate struct {
		fm     *schema.Frontmatter
		body   []byte
		branch string
		date   time.Time
	}

	var best *candidate

	for _, branch := range branches {
		fm, body, err := repo.ReadPageWithMeta(branch, path)
		if err != nil {
			continue
		}

		// Get the latest commit date for this file on this branch
		commits, _ := repo.PageHistory(branch, path, 1)
		var commitDate time.Time
		if len(commits) > 0 {
			commitDate = commits[0].Date
		}

		if best == nil || commitDate.After(best.date) {
			best = &candidate{fm: fm, body: body, branch: branch, date: commitDate}
		}
	}

	if best == nil {
		return nil, nil, "", fmt.Errorf("page not found on any branch")
	}
	return best.fm, best.body, best.branch, nil
}

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

	fm, body, pageBranch, err := readPageNewest(repo, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Get last edit info from git history
	var lastEditBy, lastEditDate string
	commits, _ := repo.PageHistoryAllBranches(path, 1)
	if len(commits) > 0 {
		lastEditBy = commits[0].Author
		lastEditDate = commits[0].Date.Format("2006-01-02 15:04")
	}

	// Render markdown body to HTML.
	var htmlBuf bytes.Buffer
	if err := goldmark.Convert(body, &htmlBuf); err != nil {
		http.Error(w, "markdown render error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	pvd := PageViewData{
		Project:      project,
		ID:           fm.ID,
		Title:        fm.Title,
		Type:         fm.Type,
		Status:       fm.Status,
		TrustLevel:   fm.TrustLevel,
		Creator:      fm.DCCreator,
		Tags:         fm.Tags,
		BodyHTML:      template.HTML(htmlBuf.String()),
		Sources:      fm.Provenance.Sources,
		Branch:       pageBranch,
		LastEditBy:   lastEditBy,
		LastEditDate: lastEditDate,
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

	fm, body, _, err := readPageNewest(repo, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
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

// searchPages renders search results for a project.
func (h *Handler) searchPages(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")

	q := r.URL.Query()
	query := q.Get("q")
	typ := q.Get("type")
	status := q.Get("status")
	tag := q.Get("tag")

	sd := SearchData{
		Query:  query,
		Type:   typ,
		Status: status,
		Tag:    tag,
	}

	// Only run a search if at least one filter is provided.
	if query != "" || typ != "" || status != "" || tag != "" {
		var items []PageListItem

		if query != "" {
			// Text queries go through the librarian → vector store
			lib, ok := h.librarians[project]
			if ok {
				vresults, err := lib.Search(r.Context(), project, query, 20)
				if err == nil {
					seen := make(map[string]bool)
					for _, vr := range vresults {
						if seen[vr.PageID] {
							continue
						}
						seen[vr.PageID] = true
						pages, err := h.search.ByID(project, vr.PageID)
						if err == nil && len(pages) > 0 {
							pr := pages[0]
							items = append(items, PageListItem{
								ID: pr.ID, Title: pr.Title, Type: pr.Type,
								Status: pr.Status, TrustLevel: pr.TrustLevel, Path: pr.Path,
								Score: fmt.Sprintf("%.0f%%", vr.Similarity*100),
							})
						}
					}
				}
			}
		} else {
			// Structured filters go to SQLite
			var results []index.PageResult
			var err error
			switch {
			case typ != "":
				results, err = h.search.ByType(project, typ)
			case status != "":
				results, err = h.search.ByStatus(project, status)
			case tag != "":
				results, err = h.search.ByTag(project, tag)
			}
			if err != nil {
				http.Error(w, "search failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
			for _, pr := range results {
				items = append(items, PageListItem{
					ID: pr.ID, Title: pr.Title, Type: pr.Type,
					Status: pr.Status, TrustLevel: pr.TrustLevel, Path: pr.Path,
				})
			}
		}

		sd.Results = items
	}

	data := PageData{
		Project:  project,
		Title:    "Search — " + project,
		Content:  sd,
		Projects: h.projects(),
	}

	t := h.templates["templates/search.html"]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// pageHistory renders the commit history for a page.
func (h *Handler) pageHistory(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")

	repo, ok := h.repos[project]
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	path := "pages/" + id + ".md"

	commits, err := repo.PageHistoryAllBranches(path, 50)
	if err != nil || len(commits) == 0 {
		http.Error(w, "no history found for this page", http.StatusNotFound)
		return
	}

	// Determine page title from the latest version.
	title := id
	fm, _, _, fmErr := readPageNewest(repo, path)
	if fmErr == nil && fm.Title != "" {
		title = fm.Title
	}

	hcs := make([]HistoryCommit, len(commits))
	for i, c := range commits {
		short := c.Hash
		if len(short) > 7 {
			short = short[:7]
		}
		hcs[i] = HistoryCommit{
			Hash:      c.Hash,
			ShortHash: short,
			Author:    c.Author,
			DateStr:   c.Date.Format("2006-01-02 15:04"),
			Message:   c.Message,
		}
	}

	phd := PageHistoryData{
		ID:      id,
		Title:   title,
		Commits: hcs,
	}

	data := PageData{
		Project:  project,
		Title:    "History: " + title,
		Content:  phd,
		Projects: h.projects(),
	}

	t := h.templates["templates/page_history.html"]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// pageAtCommitView renders a page at a specific commit.
func (h *Handler) pageAtCommitView(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")
	hash := r.PathValue("hash")

	repo, ok := h.repos[project]
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	path := "pages/" + id + ".md"
	data, err := repo.ReadPageAtCommit(hash, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Try to parse as frontmatter + markdown; fall back to raw render.
	fm, body, parseErr := schema.Parse(data)

	var htmlBuf bytes.Buffer
	if parseErr == nil {
		if err := goldmark.Convert(body, &htmlBuf); err != nil {
			http.Error(w, "markdown render error: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		if err := goldmark.Convert(data, &htmlBuf); err != nil {
			http.Error(w, "markdown render error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		fm = &schema.Frontmatter{ID: id, Title: id}
	}

	short := hash
	if len(short) > 7 {
		short = short[:7]
	}

	pvd := PageViewData{
		Project:    project,
		ID:         fm.ID,
		Title:      fm.Title + " (at " + short + ")",
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

	pageData := PageData{
		Project:  project,
		Title:    pvd.Title + " — " + project,
		Content:  pvd,
		Projects: h.projects(),
	}

	t := h.templates["templates/page_view.html"]
	if err := t.ExecuteTemplate(w, "layout", pageData); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// pageDiff renders a diff between two commits for a page.
func (h *Handler) pageDiff(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")

	repo, ok := h.repos[project]
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		http.Error(w, "from and to query params required", http.StatusBadRequest)
		return
	}

	path := "pages/" + id + ".md"

	fromData, _ := repo.ReadPageAtCommit(from, path) // empty if file didn't exist yet
	toData, _ := repo.ReadPageAtCommit(to, path)     // empty if file was deleted

	changes := computeLineDiff(string(fromData), string(toData))

	title := id
	fm, _, _, fmErr := readPageNewest(repo, path)
	if fmErr == nil && fm.Title != "" {
		title = fm.Title
	}

	fromShort, toShort := from, to
	if len(fromShort) > 7 {
		fromShort = fromShort[:7]
	}
	if len(toShort) > 7 {
		toShort = toShort[:7]
	}

	pdd := PageDiffData{
		ID:        id,
		Title:     title,
		From:      from,
		To:        to,
		FromShort: fromShort,
		ToShort:   toShort,
		Changes:   changes,
	}

	data := PageData{
		Project:  project,
		Title:    "Diff: " + title,
		Content:  pdd,
		Projects: h.projects(),
	}

	t := h.templates["templates/page_diff.html"]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// computeLineDiff produces a line-by-line diff using the LCS algorithm.
func computeLineDiff(oldText, newText string) []DiffChange {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	m, n := len(oldLines), len(newLines)
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var changes []DiffChange
	i, j := 0, 0
	for i < m && j < n {
		if oldLines[i] == newLines[j] {
			changes = append(changes, DiffChange{Type: "context", Content: oldLines[i]})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			changes = append(changes, DiffChange{Type: "delete", Content: oldLines[i]})
			i++
		} else {
			changes = append(changes, DiffChange{Type: "add", Content: newLines[j]})
			j++
		}
	}
	for ; i < m; i++ {
		changes = append(changes, DiffChange{Type: "delete", Content: oldLines[i]})
	}
	for ; j < n; j++ {
		changes = append(changes, DiffChange{Type: "add", Content: newLines[j]})
	}

	return changes
}

// splitLines splits text into lines, stripping the trailing empty line from a final newline.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

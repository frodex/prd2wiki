package web

import (
	"bytes"
	"html/template"
	"net/http"

	"github.com/frodex/prd2wiki/internal/diff"
	"github.com/frodex/prd2wiki/internal/schema"
)

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

// pageHistory renders the commit history for a page.
func (h *Handler) pageHistory(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")

	repo, ok := h.repos[project]
	if !ok {
		h.renderError(w, http.StatusNotFound, "Project not found.")
		return
	}

	path := h.resolvePagePath(project, id)

	commits, err := repo.PageHistoryAllBranches(path, 50)
	if err != nil || len(commits) == 0 {
		h.renderError(w, http.StatusNotFound, "No history found for this page.")
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
		h.renderError(w, http.StatusNotFound, "Project not found.")
		return
	}

	path := h.resolvePagePath(project, id)
	data, err := repo.ReadPageAtCommit(hash, path)
	if err != nil {
		h.renderError(w, http.StatusNotFound, "Page version not found.")
		return
	}

	// Try to parse as frontmatter + markdown; fall back to raw render.
	fm, body, parseErr := schema.Parse(data)

	var htmlBuf bytes.Buffer
	if parseErr == nil {
		if err := md.Convert(body, &htmlBuf); err != nil {
			http.Error(w, "markdown render error: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		if err := md.Convert(data, &htmlBuf); err != nil {
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
		BodyHTML:   template.HTML(sanitizeHTML(htmlBuf.String())),
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
		h.renderError(w, http.StatusNotFound, "Project not found.")
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		h.renderError(w, http.StatusBadRequest, "Both 'from' and 'to' query parameters are required.")
		return
	}

	path := h.resolvePagePath(project, id)

	fromData, _ := repo.ReadPageAtCommit(from, path) // empty if file didn't exist yet
	toData, _ := repo.ReadPageAtCommit(to, path)     // empty if file was deleted

	diffChanges := diff.ComputeLineDiff(string(fromData), string(toData))

	// Convert diff.Change to DiffChange for template compatibility.
	changes := make([]DiffChange, len(diffChanges))
	for i, dc := range diffChanges {
		changes[i] = DiffChange{Type: dc.Type, Content: dc.Content}
	}

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

	pageData := PageData{
		Project:  project,
		Title:    "Diff: " + title,
		Content:  pdd,
		Projects: h.projects(),
	}

	t := h.templates["templates/page_diff.html"]
	if err := t.ExecuteTemplate(w, "layout", pageData); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

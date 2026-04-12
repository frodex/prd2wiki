package web

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/schema"
)

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
	BodyHTML     template.HTML
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
		h.renderError(w, http.StatusNotFound, "Project not found.")
		return
	}

	if loc, ok := h.treeLegacyRedirectLocation(project, id); ok {
		http.Redirect(w, r, loc, http.StatusMovedPermanently)
		return
	}

	// Determine the page path from the index (supports subdirectories).
	path := h.resolvePagePath(project, id)

	h.viewPageAtGitPath(w, project, path, repo)
}

// viewPageAtGitPath renders a page from a resolved git path (used by /projects/... and tree routes).
func (h *Handler) viewPageAtGitPath(w http.ResponseWriter, project, gitPath string, repo *wgit.Repo) {
	fm, body, pageBranch, err := readPageNewest(repo, gitPath)
	if err != nil {
		h.renderError(w, http.StatusNotFound, "Page not found.")
		return
	}

	// Get last edit info from git history
	var lastEditBy, lastEditDate string
	commits, _ := repo.PageHistoryAllBranches(gitPath, 1)
	if len(commits) > 0 {
		lastEditBy = commits[0].Author
		lastEditDate = commits[0].Date.Format("2006-01-02 15:04")
	}

	// Render markdown body to HTML.
	var htmlBuf bytes.Buffer
	if err := md.Convert(body, &htmlBuf); err != nil {
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
		BodyHTML:     template.HTML(sanitizeHTML(htmlBuf.String())),
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

func (h *Handler) treeLegacyRedirectLocation(project, id string) (string, bool) {
	if h.treeHolder == nil || h.treeHolder.Get() == nil {
		return "", false
	}
	repo, ok := h.repos[project]
	if !ok {
		return "", false
	}
	gitPath := h.resolvePagePath(project, id)
	fm, _, _, err := readPageNewest(repo, gitPath)
	if err != nil || fm == nil {
		return "", false
	}
	uuid := strings.TrimSpace(fm.ID)
	if uuid == "" {
		return "", false
	}
	ent, ok := h.treeHolder.Get().PageByUUID(uuid)
	if !ok {
		return "", false
	}
	return "/" + ent.URLPath(), true
}

// editPage renders the page edit form with existing page content.
func (h *Handler) editPage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")

	repo, ok := h.repos[project]
	if !ok {
		h.renderError(w, http.StatusNotFound, "Project not found.")
		return
	}

	path := h.resolvePagePath(project, id)

	fm, body, _, err := readPageNewest(repo, path)
	if err != nil {
		h.renderError(w, http.StatusNotFound, "Page not found.")
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
		h.renderError(w, http.StatusNotFound, "Project not found.")
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

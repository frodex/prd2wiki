package web

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"path"
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
	// WikiURL is the on-disk tree URL when the page is indexed (e.g. /prd2wiki/foo).
	WikiURL string
	// ProjectPageURL is /projects/{project}/pages/{id} (API-style path; may redirect when followed).
	ProjectPageURL string
	OnWikiURL      bool // current request path matches WikiURL
	OnProjectPage  bool // current request path matches ProjectPageURL
}

// PageEditData holds data for the page edit template.
type PageEditData struct {
	IsNew       bool
	ID          string
	Title       string
	Type        string
	Status      string
	TagsCSV     string
	Body    string
	WikiURL string // wiki tree URL when indexed (shown on edit existing page)
}

func pageURLPathMode(r *http.Request, wikiURL, projectPageURL string) (onWiki, onProject bool) {
	if r == nil {
		return false, false
	}
	cur := path.Clean(r.URL.Path)
	if wikiURL != "" && cur == path.Clean(wikiURL) {
		return true, false
	}
	if projectPageURL != "" && cur == path.Clean(projectPageURL) {
		return false, true
	}
	return false, false
}

func (h *Handler) setPageViewURLs(pvd *PageViewData, r *http.Request) {
	pvd.ProjectPageURL = "/projects/" + pvd.Project + "/pages/" + pvd.ID
	if h.treeHolder != nil && h.treeHolder.Get() != nil {
		if ent, ok := h.treeHolder.Get().PageByUUID(pvd.ID); ok {
			pvd.WikiURL = "/" + ent.URLPath()
		}
	}
	pvd.OnWikiURL, pvd.OnProjectPage = pageURLPathMode(r, pvd.WikiURL, pvd.ProjectPageURL)
}

// readPageNewest finds the most recently modified version of a page across all branches.
// Returns the frontmatter, body, and the branch it was found on.
// aliasPaths are pre-migration paths for the same page (migration-map.json).
func readPageNewest(repo *wgit.Repo, path string, aliasPaths ...string) (*schema.Frontmatter, []byte, string, error) {
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
		commits, _ := repo.PageHistory(branch, path, 1, aliasPaths...)
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

	h.viewPageAtGitPath(w, r, project, path, repo)
}

// viewPageAtGitPath renders a page from a resolved git path (used by /projects/... and tree routes).
func (h *Handler) viewPageAtGitPath(w http.ResponseWriter, r *http.Request, project, gitPath string, repo *wgit.Repo) {
	fm, body, pageBranch, err := readPageNewest(repo, gitPath, h.aliasPathsFor(gitPath)...)
	if err != nil {
		h.renderError(w, http.StatusNotFound, "Page not found.")
		return
	}

	// Get last edit info from cache (built at startup, updated on writes).
	var lastEditBy, lastEditDate string
	if cache, ok := h.edits[project]; ok {
		if info, ok := cache.Get(gitPath); ok {
			lastEditBy = info.Author
			lastEditDate = info.Date
		}
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

	h.setPageViewURLs(&pvd, r)

	data := PageData{
		Project:     project,
		Title:       fm.Title + " — " + project,
		Content:     pvd,
		Breadcrumbs: h.breadcrumbsForGitPage(project, fm.ID, fm.Title),
	}
	h.preparePageData(&data)

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
	fm, _, _, err := readPageNewest(repo, gitPath, h.aliasPathsFor(gitPath)...)
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

	fm, body, _, err := readPageNewest(repo, path, h.aliasPathsFor(path)...)
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
	if h.treeHolder != nil && h.treeHolder.Get() != nil {
		if ent, ok := h.treeHolder.Get().PageByUUID(fm.ID); ok {
			ped.WikiURL = "/" + ent.URLPath()
		}
	}

	data := PageData{
		Project:     project,
		Title:       "Edit: " + fm.Title,
		Content:     ped,
		Breadcrumbs: projectSectionBreadcrumbs(project, "Edit"),
	}
	h.preparePageData(&data)

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
		Project:     project,
		Title:       "New Page",
		Content:     ped,
		Breadcrumbs: projectSectionBreadcrumbs(project, "New page"),
	}
	h.preparePageData(&data)

	t := h.templates["templates/page_edit.html"]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

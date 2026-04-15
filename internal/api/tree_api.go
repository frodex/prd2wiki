package api

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/schema"
	"github.com/frodex/prd2wiki/internal/tree"
	"github.com/google/uuid"
)

const apiTreePrefix = "/api/tree"

// TreeCreateRequest is the JSON body for POST /api/tree/{project}/pages.
type TreeCreateRequest struct {
	Title  string   `json:"title"`
	Slug   string   `json:"slug"`
	Type   string   `json:"type"`
	Status string   `json:"status"`
	Body   string   `json:"body"`
	Tags   []string `json:"tags"`
	Branch string   `json:"branch"`
	Intent string   `json:"intent"`
	Author string   `json:"author"`
}

// TreeUpdateRequest is the JSON body for PUT /api/tree/.../{slug}.
type TreeUpdateRequest struct {
	Title  string   `json:"title"`
	Type   string   `json:"type"`
	Status string   `json:"status"`
	Body   string   `json:"body"`
	Tags   []string `json:"tags"`
	Branch string   `json:"branch"`
	Intent string   `json:"intent"`
	Author string   `json:"author"`
}

func (s *Server) handleTreeAPI(w http.ResponseWriter, r *http.Request) {
	if s.treeHolder == nil || s.treeHolder.Get() == nil {
		http.Error(w, "tree API unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet && !s.requireWriteScope(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		// Extract project from the first segment of the tree path for logging.
		treePath := strings.TrimPrefix(path.Clean(r.URL.Path), apiTreePrefix+"/")
		treePath = strings.Trim(treePath, "/")
		proj := treePath
		if i := strings.IndexByte(proj, '/'); i >= 0 {
			proj = proj[:i]
		}
		logMutation(r, "tree", r.Method, proj)
	}
	p := path.Clean(r.URL.Path)
	if p == apiTreePrefix {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.treeListAll(w, r)
		return
	}
	if !strings.HasPrefix(p, apiTreePrefix+"/") {
		http.NotFound(w, r)
		return
	}
	rest := strings.TrimPrefix(p, apiTreePrefix+"/")
	rest = strings.Trim(rest, "/")

	switch r.Method {
	case http.MethodGet:
		s.treeGetEntry(w, r, rest)
	case http.MethodPost:
		if strings.HasSuffix(rest, "/pages") {
			s.treeCreatePage(w, r, strings.TrimSuffix(strings.TrimSuffix(rest, "/pages"), "/"))
			return
		}
		http.NotFound(w, r)
	case http.MethodPut:
		s.treeUpdatePage(w, r, rest)
	case http.MethodDelete:
		s.treeDeletePage(w, r, rest)
	default:
		w.Header().Set("Allow", "GET, POST, PUT, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) treeListAll(w http.ResponseWriter, r *http.Request) {
	idx := s.treeHolder.Get()
	out := make([]map[string]interface{}, 0, len(idx.Projects))
	for _, proj := range idx.Projects {
		pages := make([]map[string]string, 0)
		for _, e := range idx.AllPageEntries() {
			if e.Project.Path != proj.Path {
				continue
			}
			pages = append(pages, map[string]string{
				"slug": e.Page.Slug, "title": e.Page.Title, "uuid": e.Page.UUID,
				"tree_path": e.Page.TreePath,
			})
		}
		out = append(out, map[string]interface{}{
			"repo_key":     proj.RepoKey,
			"tree_path":    proj.Path,
			"uuid":         proj.UUID,
			"display_name": proj.Name,
			"pages":        pages,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"projects": out})
}

func (s *Server) treeGetEntry(w http.ResponseWriter, r *http.Request, rest string) {
	idx := s.treeHolder.Get()
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	// Single page: full tree URL path e.g. prd2wiki/my-slug
	if ent, ok := idx.PageByURLPath(rest); ok {
		s.writeTreePageJSON(w, ent)
		return
	}
	// Directory: project only e.g. prd2wiki
	if proj, ok := idx.ProjectByTreePath(rest); ok {
		pages := make([]map[string]string, 0)
		for _, e := range idx.AllPageEntries() {
			if e.Project.Path != proj.Path {
				continue
			}
			pages = append(pages, map[string]string{
				"slug": e.Page.Slug, "title": e.Page.Title, "uuid": e.Page.UUID,
				"tree_path": e.Page.TreePath,
			})
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"repo_key": proj.RepoKey, "tree_path": proj.Path, "uuid": proj.UUID,
			"display_name": proj.Name, "pages": pages,
		})
		return
	}
	http.NotFound(w, r)
}

func (s *Server) writeTreePageJSON(w http.ResponseWriter, ent *tree.PageEntry) {
	repo, ok := s.repos[ent.Project.RepoKey]
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	gitPath := "pages/" + strings.TrimSpace(ent.Page.UUID) + ".md"
	branch, err := repo.FindBranchForPage(gitPath)
	if err != nil {
		http.Error(w, "page not found in git", http.StatusNotFound)
		return
	}
	fm, body, err := repo.ReadPageWithMeta(branch, gitPath)
	if err != nil || fm == nil {
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id": fm.ID, "title": fm.Title, "type": fm.Type, "status": fm.Status,
		"tags": fm.Tags, "body": string(body), "tree_path": ent.Page.TreePath,
		"slug": ent.Page.Slug, "repo_key": ent.Project.RepoKey,
	})
}

func (s *Server) treeCreatePage(w http.ResponseWriter, r *http.Request, projectTreePath string) {
	projectTreePath = strings.Trim(projectTreePath, "/")
	idx := s.treeHolder.Get()
	proj, ok := idx.ProjectByTreePath(projectTreePath)
	if !ok {
		http.Error(w, "unknown project path", http.StatusNotFound)
		return
	}
	var req TreeCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}
	if req.Status == "" {
		req.Status = "draft"
	}
	if req.Type == "" {
		req.Type = "concept"
	}
	if req.Branch == "" {
		req.Branch = "draft/incoming"
	}
	if req.Author == "" {
		req.Author = "api@prd2wiki"
	}
	intent := req.Intent
	if intent == "" {
		intent = librarian.IntentVerbatim
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = tree.SlugFromTitle(req.Title)
	}
	used := idx.UsedSlugs(proj.Path)
	slug = tree.UniqueSlug(slug, used)

	lib, ok := s.librarians[proj.RepoKey]
	if !ok {
		http.Error(w, "project repo missing", http.StatusInternalServerError)
		return
	}

	// Allocate page UUID and write .link before git submit so async librarian sync can update line 2.
	pageUUID := uuid.New().String()
	fm := &schema.Frontmatter{
		ID:        pageUUID,
		Title:     req.Title,
		Type:      req.Type,
		Status:    req.Status,
		Tags:      req.Tags,
		DCCreator: req.Author,
		DCCreated: schema.Date{Time: time.Now().UTC()},
	}

	if err := tree.WriteLinkFile(s.treeHolder.TreeRoot(), proj.Path, slug, pageUUID, req.Title); err != nil {
		http.Error(w, "write .link: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.treeHolder.Refresh(); err != nil {
		http.Error(w, "tree refresh: "+err.Error(), http.StatusInternalServerError)
		return
	}

	res, err := lib.Submit(r.Context(), librarian.SubmitRequest{
		Project:         proj.RepoKey,
		Branch:          req.Branch,
		Frontmatter:     fm,
		Body:            []byte(req.Body),
		Intent:          intent,
		Author:          req.Author,
		UseFlatUUIDPath: true,
		ProjectUUID:     proj.UUID,
		PageUUID:        pageUUID,
	})
	if err != nil {
		_ = tree.DeleteLinkFile(s.treeHolder.TreeRoot(), proj.Path, slug)
		_ = s.treeHolder.Refresh()
		http.Error(w, "submit failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !res.Saved {
		_ = tree.DeleteLinkFile(s.treeHolder.TreeRoot(), proj.Path, slug)
		_ = s.treeHolder.Refresh()
		writeJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{"valid": false, "issues": res.Issues})
		return
	}

	if cache, ok := s.edits[proj.RepoKey]; ok && cache != nil {
		cache.Touch(res.Path, req.Author)
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id": pageUUID, "slug": slug, "title": req.Title, "tree_path": proj.Path + "/" + slug,
		"path": res.Path, "commit_hash": res.CommitHash, "url": "/" + proj.Path + "/" + slug,
	})
}

func (s *Server) treeUpdatePage(w http.ResponseWriter, r *http.Request, rest string) {
	idx := s.treeHolder.Get()
	ent, ok := idx.PageByURLPath(rest)
	if !ok {
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}
	var req TreeUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Branch == "" {
		req.Branch = "draft/incoming"
	}
	if req.Author == "" {
		req.Author = "api@prd2wiki"
	}
	intent := req.Intent
	if intent == "" {
		intent = librarian.IntentVerbatim
	}

	lib, ok := s.librarians[ent.Project.RepoKey]
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	fm := &schema.Frontmatter{
		ID:        ent.Page.UUID,
		Title:     req.Title,
		Type:      req.Type,
		Status:    req.Status,
		Tags:      req.Tags,
		DCCreator: req.Author,
	}
	if strings.TrimSpace(req.Title) == "" {
		fm.Title = ent.Page.Title
	}
	if fm.Type == "" {
		fm.Type = "concept"
	}
	if fm.Status == "" {
		fm.Status = "draft"
	}

	res, err := lib.Submit(r.Context(), librarian.SubmitRequest{
		Project:         ent.Project.RepoKey,
		Branch:          req.Branch,
		Frontmatter:     fm,
		Body:            []byte(req.Body),
		Intent:          intent,
		Author:          req.Author,
		UseFlatUUIDPath: true,
		PageUUID:        ent.Page.UUID,
		ProjectUUID:     ent.Project.UUID,
	})
	if err != nil {
		http.Error(w, "submit failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !res.Saved {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{"valid": false, "issues": res.Issues})
		return
	}

	if err := s.treeHolder.Refresh(); err != nil {
		http.Error(w, "tree refresh: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if cache, ok := s.edits[ent.Project.RepoKey]; ok && cache != nil {
		cache.Touch(res.Path, req.Author)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id": ent.Page.UUID, "slug": ent.Page.Slug, "path": res.Path, "commit_hash": res.CommitHash,
	})
}

func (s *Server) treeDeletePage(w http.ResponseWriter, r *http.Request, rest string) {
	idx := s.treeHolder.Get()
	ent, ok := idx.PageByURLPath(rest)
	if !ok {
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}
	lib, ok := s.librarians[ent.Project.RepoKey]
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	if err := tree.DeleteLinkFile(s.treeHolder.TreeRoot(), ent.Project.Path, ent.Page.Slug); err != nil {
		http.Error(w, "remove .link: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := lib.RemoveFromIndexes(ent.Page.UUID); err != nil {
		http.Error(w, "unindex: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.treeHolder.Refresh(); err != nil {
		http.Error(w, "tree refresh: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

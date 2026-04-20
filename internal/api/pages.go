package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/schema"
)

// CreatePageRequest is the JSON body for creating or updating a page.
type CreatePageRequest struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Type   string   `json:"type"`
	Status string   `json:"status"`
	Body   string   `json:"body"`
	Tags   []string `json:"tags"`
	Branch string   `json:"branch"`
	Intent string   `json:"intent"`
	Author string   `json:"author"`
}

func (s *Server) createPage(w http.ResponseWriter, r *http.Request) {
	s.upsertPage(w, r, true)
}

func (s *Server) updatePage(w http.ResponseWriter, r *http.Request) {
	s.upsertPage(w, r, false)
}

func (s *Server) upsertPage(w http.ResponseWriter, r *http.Request, isCreate bool) {
	project := r.PathValue("project")
	handler := "updatePage"
	if isCreate {
		handler = "createPage"
	}
	logMutation(r, "project", handler, project)

	if s.keys != nil && !s.requireWriteScope(w, r) {
		return
	}

	lib, ok := s.projectLibrarian(w, project)
	if !ok {
		return
	}

	var req CreatePageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Apply defaults for non-frontmatter request fields (branch / author / intent).
	// Frontmatter defaults (status, type, dc.created) are applied below only on
	// the create path — the update path preserves existing frontmatter for any
	// field the request omits. See R13-6 / T0-NEW-A.
	if req.Branch == "" {
		req.Branch = "draft/incoming"
	}
	// Capture request-provided author BEFORE defaulting for git commit author.
	// The merge path needs to distinguish "caller explicitly set dc.creator"
	// from "handler defaulted the commit author."
	fmAuthor := req.Author
	if req.Author == "" {
		req.Author = "anonymous@prd2wiki"
	}

	intent := req.Intent
	if intent == "" {
		intent = librarian.IntentVerbatim
	}

	now := time.Now().UTC()
	var fm *schema.Frontmatter

	// Update path: read-modify-write merge. A nil/absent request field
	// preserves the existing value; a non-nil non-empty request field
	// overrides. An explicit empty slice clears (tags only; null JSON is
	// indistinguishable from absent via encoding/json and preserves).
	// A PUT against a non-existent id falls through to the create path.
	if !isCreate && req.ID != "" {
		existing := s.loadExistingFrontmatter(project, req.Branch, req.ID)
		if existing != nil {
			fm = existing
			if req.Title != "" {
				fm.Title = req.Title
			}
			if req.Type != "" {
				fm.Type = req.Type
			}
			if req.Status != "" {
				fm.Status = req.Status
			}
			if req.Tags != nil {
				fm.Tags = req.Tags
			}
			if fmAuthor != "" {
				fm.DCCreator = fmAuthor
			}
			// Backfill dc.created for pre-fix pages that lacked it; otherwise preserve.
			if fm.DCCreated.Time.IsZero() {
				fm.DCCreated = schema.Date{Time: now}
			}
			fm.DCModified = schema.Date{Time: now}
		}
	}

	// Create path (isCreate=true, OR update with missing req.ID, OR update
	// against a non-existent id). Defaults apply here only.
	if fm == nil {
		if req.Status == "" {
			req.Status = "draft"
		}
		fm = &schema.Frontmatter{
			ID:         req.ID,
			Title:      req.Title,
			Type:       req.Type,
			Status:     req.Status,
			Tags:       req.Tags,
			DCCreator:  req.Author,
			DCCreated:  schema.Date{Time: now},
			DCModified: schema.Date{Time: now},
		}
	}

	useFlat := schema.IsUUIDPageID(req.ID)
	result, err := lib.Submit(r.Context(), librarian.SubmitRequest{
		Project:         project,
		Branch:          req.Branch,
		Frontmatter:     fm,
		Body:            []byte(req.Body),
		Intent:          intent,
		Author:          req.Author,
		UseFlatUUIDPath: useFlat,
	})

	if err != nil {
		http.Error(w, "submit failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if !result.Saved {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{
			"valid":  false,
			"issues": result.Issues,
		})
		return
	}

	// Update edit cache so page list shows current info without restart.
	if cache, ok := s.edits[project]; ok && cache != nil {
		cache.Touch(result.Path, req.Author)
	}

	status := http.StatusOK
	if isCreate {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]interface{}{
		"id":          fm.ID,
		"title":       fm.Title,
		"status":      fm.Status,
		"path":        result.Path,
		"issues":      result.Issues,
		"warnings":    result.Warnings,
		"commit_hash": result.CommitHash,
	})
}

func (s *Server) getPage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	repo, ok := s.projectRepo(w, project)
	if !ok {
		return
	}

	id := r.PathValue("id") // read: accept original case
	branch := r.URL.Query().Get("branch")

	// Resolve path: try index first, then hash-prefix, then flat.
	path := s.resolvePagePath(project, id)

	var fm *schema.Frontmatter
	var body []byte
	var err error

	if branch != "" {
		// Specific branch requested — try resolved path, then alternate.
		fm, body, err = repo.ReadPageWithMeta(branch, path)
		if err != nil {
			altPath := s.alternatePagePath(id, path)
			if altPath != "" {
				fm, body, err = repo.ReadPageWithMeta(branch, altPath)
			}
		}
	} else {
		// Search all branches, newest first
		branch, err = repo.FindBranchForPage(path)
		if err != nil {
			// Try alternate path format.
			altPath := s.alternatePagePath(id, path)
			if altPath != "" {
				branch, err = repo.FindBranchForPage(altPath)
				if err == nil {
					path = altPath
				}
			}
		}
		if err == nil {
			fm, body, err = repo.ReadPageWithMeta(branch, path)
		}
	}

	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "page not found", http.StatusNotFound)
			return
		}
		http.Error(w, "read page: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"id":          fm.ID,
		"title":       fm.Title,
		"type":        fm.Type,
		"status":      fm.Status,
		"trust_level": fm.TrustLevel,
		"tags":        fm.Tags,
		"provenance":  fm.Provenance,
		"body":        string(body),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) deletePage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	logMutation(r, "project", "deletePage", project)
	if s.keys != nil && !s.requireWriteScope(w, r) {
		return
	}
	repo, ok := s.projectRepo(w, project)
	if !ok {
		return
	}

	id := r.PathValue("id") // read: accept original case
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "draft/incoming"
	}

	// Resolve path: try index first, then hash-prefix, then flat.
	path := s.resolvePagePath(project, id)
	author := "anonymous@prd2wiki"
	if err := repo.DeletePage(branch, path, "delete "+id, author); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "page not found", http.StatusNotFound)
			return
		}
		http.Error(w, "delete page: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove from index.
	if err := s.indexer.RemovePage(id); err != nil {
		http.Error(w, "remove from index: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listPages(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	if _, ok := s.projectRepo(w, project); !ok {
		return
	}

	q := r.URL.Query()
	typ := q.Get("type")
	status := q.Get("status")
	tag := q.Get("tag")

	var results []interface{}
	var err error

	switch {
	case typ != "":
		var res []interface{}
		pages, e := s.search.ByType(project, typ)
		err = e
		for _, p := range pages {
			res = append(res, p)
		}
		results = res
	case status != "":
		var res []interface{}
		pages, e := s.search.ByStatus(project, status)
		err = e
		for _, p := range pages {
			res = append(res, p)
		}
		results = res
	case tag != "":
		var res []interface{}
		pages, e := s.search.ByTag(project, tag)
		err = e
		for _, p := range pages {
			res = append(res, p)
		}
		results = res
	default:
		var res []interface{}
		pages, e := s.search.ListAll(project)
		err = e
		for _, p := range pages {
			res = append(res, p)
		}
		results = res
	}

	if err != nil {
		http.Error(w, "search: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if results == nil {
		results = []interface{}{}
	}

	writeJSON(w, http.StatusOK, results)
}

// loadExistingFrontmatter reads the frontmatter of an existing page on the
// given branch and returns it. Returns nil (without error) when the page is
// absent, the project is unknown, or the read fails — callers interpret nil
// as "no existing page; treat as create." Supports both hash-prefix and flat
// path layouts via alternatePagePath (mirrors getPage). Used by upsertPage
// to implement read-modify-write merge on partial PUT (R13-6 / T0-NEW-A).
func (s *Server) loadExistingFrontmatter(project, branch, id string) *schema.Frontmatter {
	repo, ok := s.repos[project]
	if !ok {
		return nil
	}
	path := s.resolvePagePath(project, id)
	fm, _, err := repo.ReadPageWithMeta(branch, path)
	if err != nil {
		altPath := s.alternatePagePath(id, path)
		if altPath == "" {
			return nil
		}
		fm, _, err = repo.ReadPageWithMeta(branch, altPath)
		if err != nil {
			return nil
		}
	}
	return fm
}

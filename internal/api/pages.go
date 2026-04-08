package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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
	repo, ok := s.repos[project]
	if !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
		return
	}

	var req CreatePageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Apply defaults.
	if req.Status == "" {
		req.Status = "draft"
	}
	if req.Branch == "" {
		req.Branch = "draft/incoming"
	}
	if req.Author == "" {
		req.Author = "anonymous@prd2wiki"
	}

	// Build frontmatter.
	fm := &schema.Frontmatter{
		ID:        req.ID,
		Title:     req.Title,
		Type:      req.Type,
		Status:    req.Status,
		Tags:      req.Tags,
		DCCreator: req.Author,
		DCCreated: schema.Date{Time: time.Now().UTC()},
	}

	// Validate schema.
	issues := schema.Validate(fm)
	if schema.HasErrors(issues) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid":  false,
			"issues": issues,
		})
		return
	}

	// Write page to git.
	path := "pages/" + req.ID + ".md"
	message := "create " + req.ID
	if !isCreate {
		message = "update " + req.ID
	}
	if err := repo.WritePageWithMeta(req.Branch, path, fm, []byte(req.Body), message, req.Author); err != nil {
		http.Error(w, "write page: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update index.
	if err := s.indexer.IndexPage(project, req.Branch, path, fm, []byte(req.Body)); err != nil {
		http.Error(w, "index page: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Collect warnings only.
	var warnings []schema.Issue
	for _, iss := range issues {
		if iss.Severity != schema.SeverityError {
			warnings = append(warnings, iss)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":     fm.ID,
		"title":  fm.Title,
		"status": fm.Status,
		"path":   path,
		"issues": warnings,
	})
}

func (s *Server) getPage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	repo, ok := s.repos[project]
	if !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
		return
	}

	id := r.PathValue("id")
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "truth"
	}

	path := "pages/" + id + ".md"
	fm, body, err := repo.ReadPageWithMeta(branch, path)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) deletePage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	repo, ok := s.repos[project]
	if !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
		return
	}

	id := r.PathValue("id")
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "draft/incoming"
	}

	path := "pages/" + id + ".md"
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
	if _, ok := s.repos[project]; !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

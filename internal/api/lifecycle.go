package api

import (
	"net/http"
	"time"

	"github.com/frodex/prd2wiki/internal/schema"
)

func (s *Server) deprecatePage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")

	repo, ok := s.projectRepo(w, project)
	if !ok {
		return
	}

	// Find the page on any branch
	path := "pages/" + id + ".md"
	branch, err := repo.FindBranchForPage(path)
	if err != nil {
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}

	fm, body, err := repo.ReadPageWithMeta(branch, path)
	if err != nil {
		http.Error(w, "read failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fm.Status = "deprecated"
	fm.DCModified = schema.Date{Time: time.Now().UTC()}

	_, err = repo.WritePageWithMeta(branch, path, fm, body,
		"deprecate: "+fm.Title, "system@prd2wiki")
	if err != nil {
		http.Error(w, "write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update index
	_ = s.indexer.IndexPage(project, branch, path, fm, body)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":     id,
		"status": "deprecated",
	})
}

func (s *Server) approvePage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")

	repo, ok := s.projectRepo(w, project)
	if !ok {
		return
	}

	path := "pages/" + id + ".md"
	branch, err := repo.FindBranchForPage(path)
	if err != nil {
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}

	fm, body, err := repo.ReadPageWithMeta(branch, path)
	if err != nil {
		http.Error(w, "read failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fm.Status = "approved"
	fm.DCModified = schema.Date{Time: time.Now().UTC()}

	_, err = repo.WritePageWithMeta(branch, path, fm, body,
		"approve: "+fm.Title, "system@prd2wiki")
	if err != nil {
		http.Error(w, "write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.indexer.IndexPage(project, branch, path, fm, body)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":     id,
		"status": "approved",
	})
}

func (s *Server) restorePage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")

	repo, ok := s.projectRepo(w, project)
	if !ok {
		return
	}

	path := "pages/" + id + ".md"
	branch, err := repo.FindBranchForPage(path)
	if err != nil {
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}

	fm, body, err := repo.ReadPageWithMeta(branch, path)
	if err != nil {
		http.Error(w, "read failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fm.Status = "draft"
	fm.DCModified = schema.Date{Time: time.Now().UTC()}

	_, err = repo.WritePageWithMeta(branch, path, fm, body,
		"restore: "+fm.Title, "system@prd2wiki")
	if err != nil {
		http.Error(w, "write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.indexer.IndexPage(project, branch, path, fm, body)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":     id,
		"status": "draft",
	})
}

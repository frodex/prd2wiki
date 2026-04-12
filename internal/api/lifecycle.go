package api

import (
	"net/http"
	"time"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/schema"
)

// findBranchAndPathForLifecycle resolves the git path (index + hash-prefix + flat, same as GET page)
// and finds a branch that contains the file. Returns ok false if the page does not exist.
func (s *Server) findBranchAndPathForLifecycle(repo *wgit.Repo, project, id string) (branch, path string, ok bool) {
	path = s.resolvePagePath(project, id)
	b, err := repo.FindBranchForPage(path)
	if err != nil {
		alt := s.alternatePagePath(id, path)
		if alt == "" {
			return "", "", false
		}
		b, err = repo.FindBranchForPage(alt)
		if err != nil {
			return "", "", false
		}
		path = alt
	}
	return b, path, true
}

func (s *Server) deprecatePage(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")

	repo, ok := s.projectRepo(w, project)
	if !ok {
		return
	}

	branch, path, found := s.findBranchAndPathForLifecycle(repo, project, id)
	if !found {
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

	branch, path, found := s.findBranchAndPathForLifecycle(repo, project, id)
	if !found {
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

	branch, path, found := s.findBranchAndPathForLifecycle(repo, project, id)
	if !found {
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

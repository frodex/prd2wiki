package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/frodex/prd2wiki/internal/index"
)

func (s *Server) searchPages(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	if _, ok := s.repos[project]; !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
		return
	}

	q := r.URL.Query()
	query := q.Get("q")
	typ := q.Get("type")
	status := q.Get("status")
	tag := q.Get("tag")

	// Structured metadata filters go to SQLite
	if query == "" {
		var results []index.PageResult
		var err error
		switch {
		case typ != "":
			results, err = s.search.ByType(project, typ)
		case status != "":
			results, err = s.search.ByStatus(project, status)
		case tag != "":
			results, err = s.search.ByTag(project, tag)
		default:
			results, err = s.search.ListAll(project)
		}
		if err != nil {
			http.Error(w, "search: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
		return
	}

	// Text queries go through the librarian → vector store (semantic search)
	lib, ok := s.librarians[project]
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	vresults, err := lib.Search(r.Context(), project, query, 20)
	if err != nil {
		http.Error(w, "search: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Enrich vector results with metadata from SQLite
	var results []index.PageResult
	seen := make(map[string]bool)
	for _, vr := range vresults {
		if seen[vr.PageID] {
			continue
		}
		seen[vr.PageID] = true

		pages, err := s.search.ByID(project, vr.PageID)
		if err == nil && len(pages) > 0 {
			results = append(results, pages[0])
		} else {
			results = append(results, index.PageResult{
				ID:      vr.PageID,
				Title:   vr.PageID,
				Project: project,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

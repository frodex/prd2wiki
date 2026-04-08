package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (s *Server) searchPages(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	if _, ok := s.repos[project]; !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
		return
	}

	q := r.URL.Query()
	typ := q.Get("type")
	status := q.Get("status")
	tag := q.Get("tag")
	// q parameter is a placeholder for full-text search; falls back to ListAll.
	// _ = q.Get("q")

	var results []interface{}
	var err error

	switch {
	case typ != "":
		pages, e := s.search.ByType(project, typ)
		err = e
		for _, p := range pages {
			results = append(results, p)
		}
	case status != "":
		pages, e := s.search.ByStatus(project, status)
		err = e
		for _, p := range pages {
			results = append(results, p)
		}
	case tag != "":
		pages, e := s.search.ByTag(project, tag)
		err = e
		for _, p := range pages {
			results = append(results, p)
		}
	default:
		pages, e := s.search.ListAll(project)
		err = e
		for _, p := range pages {
			results = append(results, p)
		}
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

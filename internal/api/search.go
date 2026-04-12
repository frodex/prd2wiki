package api

import (
	"net/http"
	"sync"

	"github.com/frodex/prd2wiki/internal/index"
)

func (s *Server) searchPages(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	if _, ok := s.projectRepo(w, project); !ok {
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
		writeJSON(w, http.StatusOK, results)
		return
	}

	// Text queries: run SQL full-text and vector semantic search concurrently.
	lib, ok := s.projectLibrarian(w, project)
	if !ok {
		return
	}

	var (
		wg         sync.WaitGroup
		sqlResults []index.PageResult
		sqlErr     error
		vecIDs     []string
		vecErr     error
	)

	wg.Add(2)

	// SQL FTS5 search
	go func() {
		defer wg.Done()
		sqlResults, sqlErr = s.search.FullText(project, query)
	}()

	// Vector semantic search
	go func() {
		defer wg.Done()
		vresults, err := lib.Search(r.Context(), project, query, 20)
		if err != nil {
			vecErr = err
			return
		}
		seen := make(map[string]bool)
		for _, vr := range vresults {
			if !seen[vr.PageID] {
				seen[vr.PageID] = true
				vecIDs = append(vecIDs, vr.PageID)
			}
		}
	}()

	wg.Wait()

	// Merge: SQL results first (exact matches), then vector results not already seen.
	seen := make(map[string]bool)
	var results []index.PageResult

	if sqlErr == nil {
		for _, r := range sqlResults {
			seen[r.ID] = true
			results = append(results, r)
		}
	}

	if vecErr == nil {
		for _, id := range vecIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			pages, err := s.search.ByID(project, id)
			if err == nil && len(pages) > 0 {
				results = append(results, pages[0])
			} else {
				results = append(results, index.PageResult{
					ID:      id,
					Title:   id,
					Project: project,
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, results)
}

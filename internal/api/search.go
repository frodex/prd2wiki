package api

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/searchmerge"
	"github.com/frodex/prd2wiki/internal/searchsnippet"
)

// searchHitResponse is the JSON shape for text search (current page row + optional vector excerpt).
type searchHitResponse struct {
	index.PageResult
	Excerpt string `json:"excerpt,omitempty"`
}

func searchHitExcerpt(vecByID map[string]librarian.SearchResult, pageID string) string {
	v, ok := vecByID[pageID]
	if !ok {
		return ""
	}
	if v.MatchFromHistory {
		return searchsnippet.HistoryVectorExcerpt(v.HistoryCommit, v.VectorSnippet)
	}
	if v.VectorSnippet != "" {
		return searchsnippet.VectorExcerpt(v.VectorSnippet)
	}
	return ""
}

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

	// Text queries: run SQL full-text and librarian semantic search concurrently.
	lib, ok := s.projectLibrarian(w, project)
	if !ok {
		return
	}

	var (
		wg         sync.WaitGroup
		sqlResults []index.PageResult
		sqlErr     error
		vecResults []librarian.SearchResult
		vecErr     error
	)

	wg.Add(2)

	// SQL FTS5 search
	go func() {
		defer wg.Done()
		sqlResults, sqlErr = s.search.FullText(project, query)
	}()

	// Librarian semantic search (deep: includes superseded rows; best hit per page is aggregated in librarian).
	go func() {
		defer wg.Done()
		var err error
		vecResults, err = lib.Search(r.Context(), project, query, 20)
		vecErr = err
	}()

	wg.Wait()

	vecByID := make(map[string]librarian.SearchResult, len(vecResults))
	var vecIDs []string
	for _, vr := range vecResults {
		vecByID[vr.PageID] = vr
		vecIDs = append(vecIDs, vr.PageID)
	}

	if vecErr != nil {
		slog.Warn("api search: semantic/vector path failed; results are SQLite FTS only", "project", project, "error", vecErr)
	}

	ftsByID := make(map[string]index.PageResult, len(sqlResults))
	var ftsOrder []string
	if sqlErr == nil {
		for _, r := range sqlResults {
			if _, dup := ftsByID[r.ID]; dup {
				continue
			}
			ftsByID[r.ID] = r
			ftsOrder = append(ftsOrder, r.ID)
		}
	}

	var mergedIDs []string
	switch {
	case sqlErr == nil && vecErr == nil:
		mergedIDs = searchmerge.MergeRRF(ftsOrder, vecIDs, searchmerge.DefaultRRFK)
	case sqlErr == nil:
		mergedIDs = ftsOrder
	case vecErr == nil:
		mergedIDs = vecIDs
	}

	var results []index.PageResult
	for _, id := range mergedIDs {
		if pr, ok := ftsByID[id]; ok {
			results = append(results, pr)
			continue
		}
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

	results = index.RerankSearchResults(results, query)

	out := make([]searchHitResponse, len(results))
	for i, pr := range results {
		out[i] = searchHitResponse{
			PageResult: pr,
			Excerpt:    searchHitExcerpt(vecByID, pr.ID),
		}
	}
	writeJSON(w, http.StatusOK, out)
}

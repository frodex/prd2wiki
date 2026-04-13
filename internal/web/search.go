package web

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/searchmerge"
)

// SearchData holds data for the search results template.
type SearchData struct {
	Query   string
	Type    string
	Status  string
	Tag     string
	Results []PageListItem
}

// searchPages renders search results for a project.
func (h *Handler) searchPages(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")

	q := r.URL.Query()
	query := q.Get("q")
	typ := q.Get("type")
	status := q.Get("status")
	tag := q.Get("tag")

	sd := SearchData{
		Query:  query,
		Type:   typ,
		Status: status,
		Tag:    tag,
	}

	// Only run a search if at least one filter is provided.
	if query != "" || typ != "" || status != "" || tag != "" {
		var items []PageListItem

		if query != "" {
			var (
				ftsResults []index.PageResult
				ftsErr     error
				vresults   []librarian.SearchResult
				vecErr     error
			)

			ftsResults, ftsErr = h.search.FullText(project, query)
			if ftsErr != nil {
				slog.Warn("web search: FTS failed", "project", project, "error", ftsErr)
			}

			lib, haveLib := h.librarians[project]
			if haveLib {
				vresults, vecErr = lib.Search(r.Context(), project, query, 20)
				if vecErr != nil {
					slog.Warn("web search: semantic path failed", "project", project, "error", vecErr)
				}
			}

			ftsByID := make(map[string]index.PageResult, len(ftsResults))
			var ftsOrder []string
			if ftsErr == nil {
				for _, pr := range ftsResults {
					if _, dup := ftsByID[pr.ID]; dup {
						continue
					}
					ftsByID[pr.ID] = pr
					ftsOrder = append(ftsOrder, pr.ID)
				}
			}

			vecByID := make(map[string]librarian.SearchResult)
			var vecOrder []string
			if haveLib && vecErr == nil {
				seenVec := make(map[string]bool)
				for _, vr := range vresults {
					if seenVec[vr.PageID] {
						continue
					}
					seenVec[vr.PageID] = true
					vecByID[vr.PageID] = vr
					vecOrder = append(vecOrder, vr.PageID)
				}
			}

			vecOK := haveLib && vecErr == nil
			var mergedIDs []string
			switch {
			case ftsErr == nil && vecOK:
				mergedIDs = searchmerge.MergeRRF(ftsOrder, vecOrder, searchmerge.DefaultRRFK)
			case ftsErr == nil:
				mergedIDs = ftsOrder
			case vecOK:
				mergedIDs = vecOrder
			}

			for _, id := range mergedIDs {
				pr, inFts := ftsByID[id]
				if !inFts {
					pages, err := h.search.ByID(project, id)
					if err == nil && len(pages) > 0 {
						pr = pages[0]
					} else {
						pr = index.PageResult{ID: id, Title: id, Project: project}
					}
				}

				vr, inVec := vecByID[id]
				var score string
				switch {
				case inFts && inVec:
					score = fmt.Sprintf("%.0f%% [vec] + fts", vr.Similarity*100)
				case inFts:
					score = "[fts]"
				default:
					score = fmt.Sprintf("%.0f%% [vec]", vr.Similarity*100)
				}

				item := PageListItem{
					ID: pr.ID, Title: pr.Title, Type: pr.Type,
					Status: pr.Status, TrustLevel: pr.TrustLevel, Path: pr.Path,
					Score: score,
				}
				if h.treeHolder != nil && h.treeHolder.Get() != nil {
					if ent, ok := h.treeHolder.Get().PageByUUID(pr.ID); ok {
						item.TreeHref = "/" + ent.URLPath()
					}
				}
				items = append(items, item)
			}
		} else {
			// Structured filters go to SQLite
			var results []index.PageResult
			var err error
			switch {
			case typ != "":
				results, err = h.search.ByType(project, typ)
			case status != "":
				results, err = h.search.ByStatus(project, status)
			case tag != "":
				results, err = h.search.ByTag(project, tag)
			}
			if err != nil {
				http.Error(w, "search failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
			for _, pr := range results {
				item := PageListItem{
					ID: pr.ID, Title: pr.Title, Type: pr.Type,
					Status: pr.Status, TrustLevel: pr.TrustLevel, Path: pr.Path,
				}
				if h.treeHolder != nil && h.treeHolder.Get() != nil {
					if ent, ok := h.treeHolder.Get().PageByUUID(pr.ID); ok {
						item.TreeHref = "/" + ent.URLPath()
					}
				}
				items = append(items, item)
			}
		}

		sd.Results = items
	}

	data := PageData{
		Project: project,
		Title:   "Search — " + project,
		Content: sd,
		Breadcrumbs: []Breadcrumb{
			{Label: "Home", Href: "/"},
			{Label: project, Href: "/projects/" + project + "/pages"},
			{Label: "Search", Href: ""},
		},
	}
	h.preparePageData(&data)

	t := h.templates["templates/search.html"]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

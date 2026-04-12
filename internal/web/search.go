package web

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/frodex/prd2wiki/internal/index"
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
			// Try librarian semantic search first, fall back to SQLite FTS
			lib, ok := h.librarians[project]
			if ok {
				vresults, err := lib.Search(r.Context(), project, query, 20)
				if err != nil {
					slog.Warn("web search: semantic path failed; falling back to SQLite FTS", "project", project, "error", err)
				} else {
					seen := make(map[string]bool)
					for _, vr := range vresults {
						if seen[vr.PageID] {
							continue
						}
						seen[vr.PageID] = true
						pages, err := h.search.ByID(project, vr.PageID)
						if err == nil && len(pages) > 0 {
							pr := pages[0]
							item := PageListItem{
								ID: pr.ID, Title: pr.Title, Type: pr.Type,
								Status: pr.Status, TrustLevel: pr.TrustLevel, Path: pr.Path,
								Score: fmt.Sprintf("%.0f%% [vector]", vr.Similarity*100),
							}
							if h.treeHolder != nil && h.treeHolder.Get() != nil {
								if ent, ok := h.treeHolder.Get().PageByUUID(pr.ID); ok {
									item.TreeHref = "/" + ent.URLPath()
								}
							}
							items = append(items, item)
						}
					}
				}
			}
			// Fallback: if librarian search returned nothing, use SQLite FTS
			if len(items) == 0 {
				ftsResults, err := h.search.Search(project, query, typ, status, tag)
				if err == nil {
					for _, pr := range ftsResults {
						item := PageListItem{
							ID: pr.ID, Title: pr.Title, Type: pr.Type,
							Status: pr.Status, TrustLevel: pr.TrustLevel, Path: pr.Path,
							Score: "[sql]",
						}
						if h.treeHolder != nil && h.treeHolder.Get() != nil {
							if ent, ok := h.treeHolder.Get().PageByUUID(pr.ID); ok {
								item.TreeHref = "/" + ent.URLPath()
							}
						}
						items = append(items, item)
					}
				}
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

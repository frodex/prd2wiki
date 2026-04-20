package web

import (
	"fmt"
	"html/template"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"

	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/searchmerge"
	"github.com/frodex/prd2wiki/internal/searchsnippet"
)

// Tunables: promote pages many others link to; demote pages that mostly outbound-link elsewhere.
const (
	searchLinkBoostPerInlink    = 8.0
	searchLinkBoostMax          = 40.0
	searchLinkPenaltyPerOutlink = 2.0
	searchLinkPenaltyMax        = 25.0
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

	var editCache *EditCache
	if c, ok := h.edits[project]; ok {
		editCache = c
	}

	// Only run a search if at least one filter is provided.
	if query != "" || typ != "" || status != "" || tag != "" {
		var items []PageListItem

		if query != "" {
			var (
				ftsResults []index.PageResult
				ftsErr     error
				vecResults []librarian.SearchResult
				vecErr     error
			)

			ftsResults, ftsErr = h.search.FullText(project, query)
			if ftsErr != nil {
				slog.Warn("web search: FTS failed", "project", project, "error", ftsErr)
			}

			lib, haveLib := h.librarians[project]
			if haveLib {
				vecResults, vecErr = lib.Search(r.Context(), project, query, 20)
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

			vecByID := make(map[string]librarian.SearchResult, len(vecResults))
			var vecOrder []string
			if haveLib && vecErr == nil {
				for _, vr := range vecResults {
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

			var mergedPRs []index.PageResult
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
				mergedPRs = append(mergedPRs, pr)
			}
			mergedPRs = index.RerankSearchResults(mergedPRs, query)

			var linkStatsByID map[string]index.LinkStats
			if len(mergedPRs) > 0 {
				linkIDs := make([]string, 0, len(mergedPRs))
				for _, pr := range mergedPRs {
					linkIDs = append(linkIDs, pr.ID)
				}
				if m, err := h.search.LinkStatsForIDs(project, linkIDs); err == nil {
					linkStatsByID = m
				} else {
					slog.Warn("web search: link stats failed", "project", project, "error", err)
				}
			}

			var ftsSnips map[string]string
			var hitCounts map[string]int
			if ftsErr == nil && query != "" {
				ftsIDs := make([]string, 0, len(mergedPRs))
				seenID := make(map[string]bool)
				for _, pr := range mergedPRs {
					if _, ok := ftsByID[pr.ID]; !ok || seenID[pr.ID] {
						continue
					}
					seenID[pr.ID] = true
					ftsIDs = append(ftsIDs, pr.ID)
				}
				var serr error
				ftsSnips, serr = h.search.FTSSnippetsBody(project, ftsIDs, query)
				if serr != nil {
					slog.Warn("web search: fts snippet query failed", "project", project, "error", serr)
					ftsSnips = nil
				}
				// Get hit counts for all merged results
				allIDs := make([]string, 0, len(mergedPRs))
				for _, pr := range mergedPRs {
					allIDs = append(allIDs, pr.ID)
				}
				hitCounts, _ = h.search.FTSHitCounts(project, allIDs, query)
			}

			for _, pr := range mergedPRs {
				_, inFts := ftsByID[pr.ID]
				vr, inVec := vecByID[pr.ID]
				hits := 0
				if hitCounts != nil {
					hits = hitCounts[pr.ID]
				}
				var score string
				switch {
				case inFts && inVec:
					score = fmt.Sprintf("%.0f%% [vec] + fts (%d hits)", vr.Similarity*100, hits)
				case inFts:
					score = fmt.Sprintf("[fts] (%d hits)", hits)
				default:
					score = fmt.Sprintf("%.0f%% [vec]", vr.Similarity*100)
				}

				var excerpt template.HTML
				if inFts && ftsSnips != nil {
					if snip, ok := ftsSnips[pr.ID]; ok && snip != "" {
						excerpt = searchsnippet.FormatSearchExcerpt(snip, query)
					}
				}
				if excerpt == "" && inVec && vr.VectorSnippet != "" {
					if vr.MatchFromHistory {
						excerpt = searchsnippet.HistoryVectorExcerptHTML(vr.HistoryCommit, vr.VectorSnippet, query)
					} else {
						excerpt = searchsnippet.VectorExcerptHTML(vr.VectorSnippet, query)
					}
				}

				item := PageListItem{
					ID: pr.ID, Title: pr.Title, Type: pr.Type,
					Status: pr.Status, TrustLevel: pr.TrustLevel, Path: pr.Path,
					HitCount: hits, Score: score, Excerpt: excerpt,
				}
				// Scoring: MatchTier, then title-query bonus, then damped body hit count, then vector sim.
				hitScore := index.HitScore(hits)
				tier := index.MatchTier(pr.Title, pr.Tags, query)
				var tierBonus float64
				switch tier {
				case 0:
					tierBonus = 1000
				case 1:
					tierBonus = 100
				default:
					tierBonus = 0
				}
				titleBon := index.TitleMatchBonus(pr.Title, query)
				// Cap body-hit contribution for everyone so tier-3 “keyword spam” cannot outrank
				// strong title matches (tier 0/1). Tier 2/3 get a tighter cap.
				effectiveHit := hitScore
				switch {
				case tier <= 1:
					effectiveHit = math.Min(hitScore, 45)
				case tier == 2:
					effectiveHit = math.Min(hitScore, 22)
				default:
					effectiveHit = math.Min(hitScore, 12)
				}
				var scoreSort float64
				switch {
				case inFts && inVec:
					scoreSort = tierBonus + titleBon + effectiveHit + vr.Similarity + 1e-3
				case inFts:
					scoreSort = tierBonus + titleBon + effectiveHit + 0.3
				default:
					scoreSort = tierBonus + titleBon + vr.Similarity
				}
				if linkStatsByID != nil {
					if st, ok := linkStatsByID[pr.ID]; ok {
						boost := math.Min(searchLinkBoostMax, float64(st.In)*searchLinkBoostPerInlink)
						pen := math.Min(searchLinkPenaltyMax, float64(st.Out)*searchLinkPenaltyPerOutlink)
						scoreSort += boost - pen
					}
				}
				item.ScoreSort = strconv.FormatFloat(scoreSort, 'f', 8, 64)
				FillPageTimestamps(&item, pr, editCache)
				if h.treeHolder != nil && h.treeHolder.Get() != nil {
					if ent, ok := h.treeHolder.Get().PageByUUID(pr.ID); ok {
						item.TreeHref = "/" + ent.URLPath()
					}
				}
				items = append(items, item)
			}
			// Re-sort by scoreSort (tier + title bonus + capped hit score + vector) descending
			sort.SliceStable(items, func(i, j int) bool {
				si, _ := strconv.ParseFloat(items[i].ScoreSort, 64)
				sj, _ := strconv.ParseFloat(items[j].ScoreSort, 64)
				return si > sj
			})
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
				FillPageTimestamps(&item, pr, editCache)
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

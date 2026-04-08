package web

import (
	"net/http"
	"sort"
	"strings"

	"github.com/frodex/prd2wiki/internal/schema"
)

// ProjectCard represents a project on the home page.
type ProjectCard struct {
	ID          string
	Title       string
	Description string
	Status      string
	PageCount   int
	Project     string // the wiki project this lives in (e.g. "default")
}

// home renders the front page with project cards.
func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	// Query for project-type pages across all known projects.
	var cards []ProjectCard
	seen := make(map[string]bool)

	for projName := range h.repos {
		results, err := h.search.ByType(projName, "project")
		if err != nil {
			continue
		}
		for _, pr := range results {
			if seen[pr.ID] {
				continue
			}
			seen[pr.ID] = true

			// Find the project's primary tag (first tag that isn't "project")
			projTag := ""
			for _, t := range strings.Split(pr.Tags, ",") {
				t = strings.TrimSpace(t)
				if t != "" && t != "project" {
					projTag = t
					break
				}
			}
			if projTag == "" {
				projTag = schema.SanitizePathSegment(pr.Title)
			}

			taggedPages, _ := h.search.ByTag(pr.Project, projTag)

			cards = append(cards, ProjectCard{
				ID:          projTag,
				Title:       pr.Title,
				Description: pr.Tags,
				Status:      pr.Status,
				PageCount:   len(taggedPages),
				Project:     pr.Project,
			})
		}
	}

	// Sort cards by title for stable ordering.
	sort.Slice(cards, func(i, j int) bool {
		return cards[i].Title < cards[j].Title
	})

	data := PageData{
		Title:    "Projects",
		Content:  cards,
		Projects: h.projects(),
	}

	t := h.templates["templates/home.html"]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

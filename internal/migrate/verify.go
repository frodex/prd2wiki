package migrate

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/frodex/prd2wiki/internal/git"
)

// VerifyResult holds the outcome of a post-migration verification.
type VerifyResult struct {
	Total        int
	HistoryOK    int
	HistoryBad   int
	CrossRefsBad int
	Errors       []string
}

func (r *VerifyResult) OK() bool {
	return r.HistoryBad == 0 && r.CrossRefsBad == 0 && len(r.Errors) == 0
}

// Verify checks that migration was successful:
// 1. git log --follow on each new path shows >1 commit (history preserved)
// 2. No old-style cross-references remain in page content
func Verify(plan *Plan) (*VerifyResult, error) {
	result := &VerifyResult{}

	for projName, proj := range plan.Projects {
		repo, err := git.OpenRepoAt(proj.RepoPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("open repo %s: %v", projName, err))
			continue
		}

		var pages []*PagePlan
		for _, p := range plan.Pages {
			if strings.HasPrefix(p.TreePath, proj.TreePath+"/") {
				pages = append(pages, p)
			}
		}

		for _, page := range pages {
			result.Total++

			// Check history: need commits on new path + old path (via follow)
			newHistory, err := repo.PageHistoryAllBranches(page.NewPath, 5)
			if err != nil || len(newHistory) == 0 {
				result.HistoryBad++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: no commits at %s", page.OldID, page.NewPath))
				continue
			}

			// Also check old path for pre-migration history
			oldHistory, _ := repo.PageHistoryAllBranches(page.OldPath, 5)
			totalCommits := len(newHistory) + len(oldHistory)

			// Dedupe by hash
			seen := make(map[string]bool)
			for _, c := range newHistory {
				seen[c.Hash] = true
			}
			for _, c := range oldHistory {
				if !seen[c.Hash] {
					totalCommits++ // don't double count
				}
			}

			if len(newHistory) <= 1 && len(oldHistory) == 0 {
				// Only the migration commit — history not preserved
				result.HistoryBad++
				result.Errors = append(result.Errors,
					fmt.Sprintf("%s: only %d commit(s) — history may not be preserved", page.OldID, totalCommits))
			} else {
				result.HistoryOK++
			}

			// Check cross-references in current content
			content, err := repo.ReadPage(page.Branch, page.NewPath)
			if err != nil {
				continue
			}
			for oldID := range plan.Pages {
				for pName := range plan.Projects {
					oldRef := fmt.Sprintf("/projects/%s/pages/%s", pName, oldID)
					if strings.Contains(string(content), oldRef) {
						result.CrossRefsBad++
						result.Errors = append(result.Errors,
							fmt.Sprintf("%s: still contains old ref %s", page.OldID, oldRef))
						break
					}
				}
			}
		}
	}

	// Log summary
	slog.Info("verification complete",
		"total", result.Total,
		"history_ok", result.HistoryOK,
		"history_bad", result.HistoryBad,
		"crossrefs_bad", result.CrossRefsBad,
		"errors", len(result.Errors))

	return result, nil
}

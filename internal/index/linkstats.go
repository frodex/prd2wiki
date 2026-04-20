package index

import (
	"fmt"
	"strings"

	wgit "github.com/frodex/prd2wiki/internal/git"
)

// LinkStats holds per-page inbound / outbound wiki link counts used for search ranking.
type LinkStats struct {
	In  int
	Out int
}

// LinkStatsForIDs returns inlink/outlink counts for the given page ids (project scope).
func (s *Searcher) LinkStatsForIDs(project string, ids []string) (map[string]LinkStats, error) {
	out := make(map[string]LinkStats, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, 0, 1+len(ids))
	args = append(args, project)
	for i, id := range ids {
		ph[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(
		`SELECT id, COALESCE(inlink_count, 0), COALESCE(outlink_count, 0) FROM pages WHERE project = ? AND id IN (%s)`,
		strings.Join(ph, ","))
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("link stats: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var inC, outC int
		if err := rows.Scan(&id, &inC, &outC); err != nil {
			return nil, err
		}
		out[id] = LinkStats{In: inC, Out: outC}
	}
	return out, rows.Err()
}

// RecomputeLinkStats walks all pages on the branch and sets pages.inlink_count / outlink_count
// for the project from markdown /pages/<id> link targets. Call after indexing bodies (e.g. end of RebuildFromRepo).
func (ix *Indexer) RecomputeLinkStats(project, branch string, repo *wgit.Repo) error {
	paths, err := repo.ListPages(branch)
	if err != nil {
		return err
	}
	outCount := make(map[string]int)
	// key: lowercase page id for stable aggregation
	inCount := make(map[string]int)

	for _, path := range paths {
		if !strings.HasSuffix(path, ".md") {
			continue
		}
		fm, body, err := repo.ReadPageWithMeta(branch, path)
		if err != nil || fm == nil {
			continue
		}
		src := fm.ID
		targets := WikiPageIDsInMarkdown(string(body))
		outCount[src] = len(targets)
		for _, tid := range targets {
			if strings.EqualFold(tid, src) {
				continue
			}
			inCount[strings.ToLower(tid)]++
		}
	}

	if _, err := ix.db.Exec(`UPDATE pages SET inlink_count = 0, outlink_count = 0 WHERE project = ?`, project); err != nil {
		return err
	}

	for id, n := range outCount {
		if _, err := ix.db.Exec(`UPDATE pages SET outlink_count = ? WHERE project = ? AND id = ?`, n, project, id); err != nil {
			return err
		}
	}
	for low, n := range inCount {
		if _, err := ix.db.Exec(
			`UPDATE pages SET inlink_count = ? WHERE project = ? AND lower(id) = ?`,
			n, project, low,
		); err != nil {
			return err
		}
	}
	return nil
}

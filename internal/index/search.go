package index

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// FTS BM25 column weights (title/tags boosted vs body). Tuned for wiki-style queries.
const (
	ftsBM25WeightTitle = 25.0
	ftsBM25WeightBody  = 1.0
	ftsBM25WeightTags  = 4.0
)

// PageResult holds the fields returned by search queries.
type PageResult struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Path       string `json:"path"`
	Project    string `json:"project"`
	TrustLevel int    `json:"trust_level"`
	Tags       string `json:"tags"`
	Module     string `json:"module"`
	Category   string `json:"category"`
	// UpdatedAt is from SQLite pages.updated_at (typically "2006-01-02 15:04:05" UTC).
	UpdatedAt string `json:"updated_at,omitempty"`
}

// Searcher queries the SQLite index for pages.
type Searcher struct {
	db *sql.DB
}

// NewSearcher creates a Searcher backed by the given database.
func NewSearcher(db *sql.DB) *Searcher {
	return &Searcher{db: db}
}

// query is the private helper that executes a SQL query and scans rows into []PageResult.
func (s *Searcher) query(sqlStr string, args ...interface{}) ([]PageResult, error) {
	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var results []PageResult
	for rows.Next() {
		var r PageResult
		if err := rows.Scan(&r.ID, &r.Title, &r.Type, &r.Status, &r.Path, &r.Project, &r.TrustLevel, &r.Tags, &r.Module, &r.Category, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return results, nil
}

const selectPagesCols = `pages.id, pages.title, pages.type, pages.status, pages.path, pages.project, pages.trust_level, COALESCE(pages.tags, ''), COALESCE(pages.module, ''), COALESCE(pages.category, ''), IFNULL(pages.updated_at, '')`

const selectPages = `SELECT ` + selectPagesCols + ` FROM pages`

// ListAll returns all pages for a given project.
func (s *Searcher) ListAll(project string) ([]PageResult, error) {
	return s.query(selectPages+` WHERE project = ?`, project)
}

// ByID returns a page by its ID within a project.
func (s *Searcher) ByID(project, id string) ([]PageResult, error) {
	return s.query(selectPages+` WHERE project = ? AND id = ?`, project, id)
}

// ByType returns pages of a given type within a project.
func (s *Searcher) ByType(project, typ string) ([]PageResult, error) {
	return s.query(selectPages+` WHERE project = ? AND type = ?`, project, typ)
}

// ByStatus returns pages with a given status within a project.
func (s *Searcher) ByStatus(project, status string) ([]PageResult, error) {
	return s.query(selectPages+` WHERE project = ? AND status = ?`, project, status)
}

// ByModule returns pages with a given module within a project.
func (s *Searcher) ByModule(project, module string) ([]PageResult, error) {
	return s.query(selectPages+` WHERE project = ? AND module = ?`, project, module)
}

// ByTag returns pages whose tags column contains the given tag (LIKE match).
func (s *Searcher) ByTag(project, tag string) ([]PageResult, error) {
	return s.query(selectPages+` WHERE project = ? AND tags LIKE ?`, project, "%"+tag+"%")
}

// sanitizeFTSQuery prepares a user query for FTS5 MATCH.
// - Replaces hyphens with spaces (FTS5 unicode61 tokenizer splits on hyphens)
// - Strips FTS5 operators that could cause syntax errors
// - Trims whitespace
func sanitizeFTSQuery(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return q
	}
	// Replace hyphens with spaces so "go-git" matches pages containing "go" and "git"
	q = strings.ReplaceAll(q, "-", " ")
	// Collapse multiple spaces
	for strings.Contains(q, "  ") {
		q = strings.ReplaceAll(q, "  ", " ")
	}
	return strings.TrimSpace(q)
}

// looksLikePageID returns true if the query looks like a page ID (hex hash or UUID).
func looksLikePageID(q string) bool {
	q = strings.TrimSpace(q)
	if q == "" {
		return false
	}
	// UUID format: 8-4-4-4-12 hex
	if len(q) == 36 && q[8] == '-' && q[13] == '-' {
		return true
	}
	// Short hex hash (5-12 chars, all hex)
	if len(q) >= 5 && len(q) <= 12 {
		for _, c := range q {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
		return true
	}
	return false
}

// FullText searches the FTS5 index for pages matching the query within a project.
// Rows are ordered by title-token relevance (all query terms in title first), then BM25, then shorter title.
// If the query looks like a page ID, a direct ID lookup is prepended to the results.
func (s *Searcher) FullText(project, q string) ([]PageResult, error) {
	var idResult []PageResult
	if looksLikePageID(q) {
		if pages, err := s.ByID(project, q); err == nil && len(pages) > 0 {
			idResult = pages
		}
	}

	ftsQ := sanitizeFTSQuery(q)
	if ftsQ == "" {
		return idResult, nil
	}

	sql := fmt.Sprintf(`SELECT %s,
		bm25(pages_fts, 'title', %g, 'body', %g, 'tags', %g) AS fts_bm25
		FROM pages
		INNER JOIN pages_fts ON pages.id = pages_fts.id
		WHERE pages.project = ? AND pages_fts MATCH ?`,
		selectPagesCols, ftsBM25WeightTitle, ftsBM25WeightBody, ftsBM25WeightTags)

	rows, err := s.db.Query(sql, project, ftsQ)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	type scored struct {
		PageResult
		bm25 float64
	}
	var buf []scored
	for rows.Next() {
		var r scored
		if err := rows.Scan(&r.ID, &r.Title, &r.Type, &r.Status, &r.Path, &r.Project, &r.TrustLevel, &r.Tags, &r.Module, &r.Category, &r.UpdatedAt, &r.bm25); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		buf = append(buf, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	sort.SliceStable(buf, func(i, j int) bool {
		ti := MatchTier(buf[i].Title, buf[i].Tags, q)
		tj := MatchTier(buf[j].Title, buf[j].Tags, q)
		if ti != tj {
			return ti < tj
		}
		if buf[i].bm25 != buf[j].bm25 {
			return buf[i].bm25 < buf[j].bm25
		}
		return len(buf[i].Title) < len(buf[j].Title)
	})

	out := make([]PageResult, 0, len(idResult)+len(buf))
	seen := make(map[string]bool)
	// Prepend ID match (if any) as the top result
	for _, pr := range idResult {
		out = append(out, pr)
		seen[pr.ID] = true
	}
	// Append FTS results, skipping duplicates
	for i := range buf {
		if !seen[buf[i].ID] {
			out = append(out, buf[i].PageResult)
		}
	}
	return out, nil
}

// FTSSnippetsBody returns FTS5 body-column snippets for pages that match matchQuery (plain text, no HTML).
func (s *Searcher) FTSSnippetsBody(project string, pageIDs []string, matchQuery string) (map[string]string, error) {
	out := make(map[string]string)
	matchQuery = sanitizeFTSQuery(matchQuery)
	if len(pageIDs) == 0 || matchQuery == "" {
		return out, nil
	}
	seen := make(map[string]bool)
	var ids []string
	for _, id := range pageIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return out, nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, 0, 2+len(ids))
	args = append(args, project, matchQuery)
	for i, id := range ids {
		ph[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(`
		SELECT pages_fts.id, snippet(pages_fts, 2, '', '', ' … ', 32)
		FROM pages_fts
		INNER JOIN pages ON pages.id = pages_fts.id
		WHERE pages.project = ? AND pages_fts MATCH ? AND pages_fts.id IN (%s)`,
		strings.Join(ph, ","))
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("fts snippet: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, snip string
		if err := rows.Scan(&id, &snip); err != nil {
			return nil, fmt.Errorf("fts snippet scan: %w", err)
		}
		if snip != "" {
			out[id] = snip
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// RerankSearchResults stable-sorts by MatchTier (title+tags vs query), then original order.
// Use after hybrid RRF so vector-heavy ordering does not bury pages that match all query terms in metadata.
func RerankSearchResults(results []PageResult, query string) []PageResult {
	if len(results) < 2 {
		return results
	}
	type slot struct {
		pr   PageResult
		ord  int
		tier int
	}
	xs := make([]slot, len(results))
	for i, pr := range results {
		xs[i] = slot{pr: pr, ord: i, tier: MatchTier(pr.Title, pr.Tags, query)}
	}
	sort.SliceStable(xs, func(i, j int) bool {
		if xs[i].tier != xs[j].tier {
			return xs[i].tier < xs[j].tier
		}
		return xs[i].ord < xs[j].ord
	})
	out := make([]PageResult, len(xs))
	for i := range xs {
		out[i] = xs[i].pr
	}
	return out
}

// Search dispatches to the appropriate query method based on the provided filters.
// It tries full-text first (if q is non-empty), then type, status, tag filters,
// and falls back to ListAll.
func (s *Searcher) Search(project, q, typ, status, tag string) ([]PageResult, error) {
	switch {
	case q != "":
		return s.FullText(project, q)
	case typ != "":
		return s.ByType(project, typ)
	case status != "":
		return s.ByStatus(project, status)
	case tag != "":
		return s.ByTag(project, tag)
	default:
		return s.ListAll(project)
	}
}

// DependentsOf returns all pages that cite the given reference via a provenance edge.
func (s *Searcher) DependentsOf(ref string) ([]PageResult, error) {
	sql := selectPages + `
		INNER JOIN provenance_edges pe ON pages.id = pe.source_page
		WHERE pe.target_ref = ?`
	return s.query(sql, ref)
}

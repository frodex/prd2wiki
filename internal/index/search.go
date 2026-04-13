package index

import (
	"database/sql"
	"fmt"
	"sort"
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
		if err := rows.Scan(&r.ID, &r.Title, &r.Type, &r.Status, &r.Path, &r.Project, &r.TrustLevel, &r.Tags, &r.Module, &r.Category); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return results, nil
}

const selectPagesCols = `pages.id, pages.title, pages.type, pages.status, pages.path, pages.project, pages.trust_level, COALESCE(pages.tags, ''), COALESCE(pages.module, ''), COALESCE(pages.category, '')`

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

// FullText searches the FTS5 index for pages matching the query within a project.
// Rows are ordered by title-token relevance (all query terms in title first), then BM25, then shorter title.
func (s *Searcher) FullText(project, q string) ([]PageResult, error) {
	sql := fmt.Sprintf(`SELECT %s,
		bm25(pages_fts, 'title', %g, 'body', %g, 'tags', %g) AS fts_bm25
		FROM pages
		INNER JOIN pages_fts ON pages.id = pages_fts.id
		WHERE pages.project = ? AND pages_fts MATCH ?`,
		selectPagesCols, ftsBM25WeightTitle, ftsBM25WeightBody, ftsBM25WeightTags)

	rows, err := s.db.Query(sql, project, q)
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
		if err := rows.Scan(&r.ID, &r.Title, &r.Type, &r.Status, &r.Path, &r.Project, &r.TrustLevel, &r.Tags, &r.Module, &r.Category, &r.bm25); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		buf = append(buf, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	sort.SliceStable(buf, func(i, j int) bool {
		ti := fullTextTitleTier(buf[i].Title, q)
		tj := fullTextTitleTier(buf[j].Title, q)
		if ti != tj {
			return ti < tj
		}
		if buf[i].bm25 != buf[j].bm25 {
			return buf[i].bm25 < buf[j].bm25
		}
		return len(buf[i].Title) < len(buf[j].Title)
	})

	out := make([]PageResult, len(buf))
	for i := range buf {
		out[i] = buf[i].PageResult
	}
	return out, nil
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

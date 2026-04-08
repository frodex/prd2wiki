package index

import (
	"database/sql"
	"fmt"
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
		if err := rows.Scan(&r.ID, &r.Title, &r.Type, &r.Status, &r.Path, &r.Project, &r.TrustLevel, &r.Tags); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return results, nil
}

const selectPages = `SELECT pages.id, pages.title, pages.type, pages.status, pages.path, pages.project, pages.trust_level, COALESCE(pages.tags, '') FROM pages`

// ListAll returns all pages for a given project.
func (s *Searcher) ListAll(project string) ([]PageResult, error) {
	return s.query(selectPages+` WHERE project = ?`, project)
}

// ByType returns pages of a given type within a project.
func (s *Searcher) ByType(project, typ string) ([]PageResult, error) {
	return s.query(selectPages+` WHERE project = ? AND type = ?`, project, typ)
}

// ByStatus returns pages with a given status within a project.
func (s *Searcher) ByStatus(project, status string) ([]PageResult, error) {
	return s.query(selectPages+` WHERE project = ? AND status = ?`, project, status)
}

// ByTag returns pages whose tags column contains the given tag (LIKE match).
func (s *Searcher) ByTag(project, tag string) ([]PageResult, error) {
	return s.query(selectPages+` WHERE project = ? AND tags LIKE ?`, project, "%"+tag+"%")
}

// DependentsOf returns all pages that cite the given reference via a provenance edge.
func (s *Searcher) DependentsOf(ref string) ([]PageResult, error) {
	sql := selectPages + `
		INNER JOIN provenance_edges pe ON pages.id = pe.source_page
		WHERE pe.target_ref = ?`
	return s.query(sql, ref)
}

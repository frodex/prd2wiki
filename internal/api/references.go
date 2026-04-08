package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// RefNode represents a node in the provenance reference tree.
type RefNode struct {
	Ref        string    `json:"ref"`
	Title      string    `json:"title,omitempty"`
	Version    int       `json:"version,omitempty"`
	Checksum   string    `json:"checksum,omitempty"`
	Status     string    `json:"status"`
	TrustLevel int       `json:"trust_level,omitempty"`
	Children   []RefNode `json:"children"`
}

func (s *Server) getReferences(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	if _, ok := s.repos[project]; !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
		return
	}

	pageID := r.PathValue("id")

	// Parse depth parameter (default 1, max 5).
	depth := 1
	if ds := r.URL.Query().Get("depth"); ds != "" {
		d, err := strconv.Atoi(ds)
		if err != nil || d < 1 {
			http.Error(w, "depth must be a positive integer", http.StatusBadRequest)
			return
		}
		if d > 5 {
			d = 5
		}
		depth = d
	}

	// Build the reference tree starting from the page.
	children, err := s.buildRefTree(pageID, depth)
	if err != nil {
		http.Error(w, "build ref tree: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// The root node represents the page itself.
	root := RefNode{
		Ref:      pageID,
		Status:   "root",
		Children: children,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(root)
}

// buildRefTree recursively queries provenance_edges for a page and builds the
// reference tree up to the specified remaining depth.
func (s *Server) buildRefTree(pageID string, remaining int) ([]RefNode, error) {
	if remaining <= 0 {
		return nil, nil
	}

	rows, err := s.db.Query(`
		SELECT target_ref, target_version, target_checksum, status
		FROM provenance_edges
		WHERE source_page = ?
	`, pageID)
	if err != nil {
		return nil, fmt.Errorf("query provenance_edges for %q: %w", pageID, err)
	}
	defer rows.Close()

	var nodes []RefNode
	for rows.Next() {
		var n RefNode
		var version int
		var checksum string
		if err := rows.Scan(&n.Ref, &version, &checksum, &n.Status); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		n.Version = version
		n.Checksum = checksum

		// Look up title and trust_level from the pages table if this ref is a page.
		row := s.db.QueryRow(`SELECT title, trust_level FROM pages WHERE id = ?`, n.Ref)
		var title string
		var trustLevel int
		if err := row.Scan(&title, &trustLevel); err == nil {
			n.Title = title
			n.TrustLevel = trustLevel
		}

		// Recurse for children.
		children, err := s.buildRefTree(n.Ref, remaining-1)
		if err != nil {
			return nil, err
		}
		n.Children = children
		if n.Children == nil {
			n.Children = []RefNode{}
		}

		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	if nodes == nil {
		nodes = []RefNode{}
	}
	return nodes, nil
}

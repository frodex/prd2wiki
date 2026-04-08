package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// DiffChange represents a single line in a unified diff.
type DiffChange struct {
	Type    string `json:"type"`    // "context", "add", "delete"
	Content string `json:"content"` // the line text
}

// DiffResult is the JSON response for a diff request.
type DiffResult struct {
	From    string       `json:"from"`
	To      string       `json:"to"`
	Changes []DiffChange `json:"changes"`
}

func (s *Server) pageHistory(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	repo, ok := s.repos[project]
	if !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
		return
	}

	id := r.PathValue("id")
	path := "pages/" + id + ".md"

	branch, err := repo.FindBranchForPage(path)
	if err != nil {
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}

	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}

	commits, err := repo.PageHistory(branch, path, limit)
	if err != nil {
		http.Error(w, "history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(commits)
}

func (s *Server) pageAtCommit(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	repo, ok := s.repos[project]
	if !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
		return
	}

	id := r.PathValue("id")
	hash := r.PathValue("hash")
	path := "pages/" + id + ".md"

	data, err := repo.ReadPageAtCommit(hash, path)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "read: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"hash":    hash,
		"content": string(data),
	})
}

func (s *Server) pageDiff(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	repo, ok := s.repos[project]
	if !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
		return
	}

	id := r.PathValue("id")
	path := "pages/" + id + ".md"
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	if from == "" || to == "" {
		http.Error(w, "from and to query params required", http.StatusBadRequest)
		return
	}

	fromData, err := repo.ReadPageAtCommit(from, path)
	if err != nil {
		http.Error(w, "read from: "+err.Error(), http.StatusNotFound)
		return
	}
	toData, err := repo.ReadPageAtCommit(to, path)
	if err != nil {
		http.Error(w, "read to: "+err.Error(), http.StatusNotFound)
		return
	}

	changes := lineDiff(string(fromData), string(toData))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(DiffResult{
		From:    from,
		To:      to,
		Changes: changes,
	})
}

// lineDiff computes a line-by-line diff using the LCS algorithm.
func lineDiff(oldText, newText string) []DiffChange {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	// Build LCS table.
	m, n := len(oldLines), len(newLines)
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	// Walk LCS table to produce diff.
	var changes []DiffChange
	i, j := 0, 0
	for i < m && j < n {
		if oldLines[i] == newLines[j] {
			changes = append(changes, DiffChange{Type: "context", Content: oldLines[i]})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			changes = append(changes, DiffChange{Type: "delete", Content: oldLines[i]})
			i++
		} else {
			changes = append(changes, DiffChange{Type: "add", Content: newLines[j]})
			j++
		}
	}
	for ; i < m; i++ {
		changes = append(changes, DiffChange{Type: "delete", Content: oldLines[i]})
	}
	for ; j < n; j++ {
		changes = append(changes, DiffChange{Type: "add", Content: newLines[j]})
	}

	return changes
}

// splitLines splits text into lines, handling empty input.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	// Remove trailing empty line from final newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

package diff

import "strings"

// Change represents one line in a diff.
type Change struct {
	Type    string // "context", "add", "delete"
	Content string
}

// ComputeLineDiff produces a line-by-line diff using the LCS algorithm.
func ComputeLineDiff(oldText, newText string) []Change {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

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

	var changes []Change
	i, j := 0, 0
	for i < m && j < n {
		if oldLines[i] == newLines[j] {
			changes = append(changes, Change{Type: "context", Content: oldLines[i]})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			changes = append(changes, Change{Type: "delete", Content: oldLines[i]})
			i++
		} else {
			changes = append(changes, Change{Type: "add", Content: newLines[j]})
			j++
		}
	}
	for ; i < m; i++ {
		changes = append(changes, Change{Type: "delete", Content: oldLines[i]})
	}
	for ; j < n; j++ {
		changes = append(changes, Change{Type: "add", Content: newLines[j]})
	}

	return changes
}

// splitLines splits text into lines, stripping the trailing empty line from a final newline.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

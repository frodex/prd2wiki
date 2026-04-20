// Package migrate handles history-preserving page migration from old hash-prefix
// IDs to UUID-based flat paths. Designed as repeatable tooling, not a one-time script.
//
// Initial tree layout uses tree.WriteProjectUUIDFile and tree.WriteLinkFile. Incremental
// moves/renames on disk should use tree.MovePage and tree.RenamePage; those operations
// are not duplicated here.
package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/schema"
	"github.com/google/uuid"
)

// PagePlan describes the migration for a single page.
type PagePlan struct {
	OldID       string    `json:"old_id"`
	UUID        string    `json:"uuid"`
	OldPath     string    `json:"old_path"`
	NewPath     string    `json:"new_path"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	TreePath    string    `json:"tree_path"`
	FirstCommit time.Time `json:"first_commit"`
	LastCommit  time.Time `json:"last_commit"`
	Branch      string    `json:"branch"`
}

// ProjectPlan describes the migration for a project.
type ProjectPlan struct {
	OldName     string `json:"old_name"`
	UUID        string `json:"uuid"`
	TreePath    string `json:"tree_path"`
	DisplayName string `json:"display_name"`
	RepoPath    string `json:"repo_path"`
}

// Plan holds the full migration plan — built before any changes are made.
type Plan struct {
	Pages    map[string]*PagePlan    `json:"pages"`    // keyed by old ID
	Projects map[string]*ProjectPlan `json:"projects"` // keyed by old project name
	DataDir  string                  `json:"data_dir"`
	TreeDir  string                  `json:"tree_dir"`
}

// ProjectConfig maps old project names to tree paths and display names.
type ProjectConfig struct {
	OldName     string
	TreePath    string // e.g. "prd2wiki" or "games/battletech"
	DisplayName string
}

// BuildPlan scans existing repos and builds a migration plan without changing anything.
func BuildPlan(dataDir string, treeDir string, projects []ProjectConfig) (*Plan, error) {
	plan := &Plan{
		Pages:    make(map[string]*PagePlan),
		Projects: make(map[string]*ProjectPlan),
		DataDir:  dataDir,
		TreeDir:  treeDir,
	}

	for _, pc := range projects {
		projUUID := uuid.New().String()
		repoName := pc.OldName + ".wiki.git"
		repoPath := filepath.Join(dataDir, repoName)

		// Check if this is a symlink to repos/ — follow it
		if target, err := os.Readlink(repoPath); err == nil {
			repoPath = filepath.Join(dataDir, target)
		}

		plan.Projects[pc.OldName] = &ProjectPlan{
			OldName:     pc.OldName,
			UUID:        projUUID,
			TreePath:    pc.TreePath,
			DisplayName: pc.DisplayName,
			RepoPath:    repoPath,
		}

		repo, err := git.OpenRepoAt(repoPath)
		if err != nil {
			return nil, fmt.Errorf("open repo %s: %w", repoPath, err)
		}

		// Find all pages on all branches
		branches, err := repo.ListBranches()
		if err != nil {
			return nil, fmt.Errorf("list branches %s: %w", pc.OldName, err)
		}

		for _, branch := range branches {
			pages, err := repo.ListPages(branch)
			if err != nil {
				continue
			}
			for _, pagePath := range pages {
				oldID := extractOldID(pagePath)
				if oldID == "" || plan.Pages[oldID] != nil {
					continue
				}

				// Read frontmatter for title
				content, err := repo.ReadPage(branch, pagePath)
				if err != nil {
					continue
				}
				fm, _, err := schema.Parse(content)
				if err != nil || fm == nil {
					continue
				}

				// Get dates from git history
				firstDate, _ := repo.FirstCommitDate(pagePath)
				lastDate := time.Now()
				history, err := repo.PageHistoryAllBranches(pagePath, 1)
				if err == nil && len(history) > 0 {
					lastDate = history[0].Date
				}

				pageUUID := uuid.New().String()
				title := fm.Title
				if title == "" {
					title = oldID
				}
				slug := slugify(title)

				plan.Pages[oldID] = &PagePlan{
					OldID:       oldID,
					UUID:        pageUUID,
					OldPath:     pagePath,
					NewPath:     fmt.Sprintf("pages/%s.md", pageUUID),
					Title:       title,
					Slug:        slug,
					TreePath:    pc.TreePath + "/" + slug,
					FirstCommit: firstDate,
					LastCommit:  lastDate,
					Branch:      branch,
				}
			}
		}
	}

	return plan, nil
}

// SavePlan writes the plan to a JSON file.
func SavePlan(plan *Plan, path string) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ProjectForTreePath returns the project whose TreePath is the longest prefix of fullTreePath
// (e.g. page "games/battletech/foo" matches nested project "games/battletech", not "games").
func (plan *Plan) ProjectForTreePath(fullTreePath string) *ProjectPlan {
	fullTreePath = strings.Trim(fullTreePath, "/")
	var best *ProjectPlan
	for _, proj := range plan.Projects {
		prefix := proj.TreePath + "/"
		if !strings.HasPrefix(fullTreePath, prefix) {
			continue
		}
		if best == nil || len(proj.TreePath) > len(best.TreePath) {
			best = proj
		}
	}
	return best
}

// LoadPlan reads a plan from a JSON file.
func LoadPlan(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

// extractOldID pulls the page ID from a git path like "pages/86/34f02.md" or "pages/8634f02.md"
func extractOldID(pagePath string) string {
	// Remove "pages/" prefix and ".md" suffix
	s := strings.TrimPrefix(pagePath, "pages/")
	s = strings.TrimSuffix(s, ".md")

	// Handle hash-prefix: "86/34f02" → "8634f02"
	parts := strings.Split(s, "/")
	if len(parts) == 2 && len(parts[0]) == 2 {
		return parts[0] + parts[1]
	}
	// Already flat
	if len(parts) == 1 {
		return parts[0]
	}
	return ""
}

// slugify converts a title to a URL-safe slug.
func slugify(title string) string {
	s := strings.ToLower(title)
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == ' ', r == '-', r == '_', r == '.', r == '/':
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	result := b.String()
	return strings.TrimRight(result, "-")
}

package migrate

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/schema"
	"github.com/frodex/prd2wiki/internal/tree"
)

// Execute runs the migration plan. It modifies git repos, creates the tree directory,
// and sets up repo symlinks. Designed to run against a COPY of the data, not the live wiki.
func Execute(plan *Plan) error {
	// Count total pages for progress
	totalPages := len(plan.Pages)

	// Phase 1: Git renames + frontmatter updates (per project repo)
	progress := &Progress{Total: totalPages}

	for projName, proj := range plan.Projects {
		slog.Info("migrating project", "name", projName, "repo", proj.RepoPath)

		repo, err := git.OpenRepoAt(proj.RepoPath)
		if err != nil {
			return fmt.Errorf("open repo %s: %w", projName, err)
		}

		// Collect pages for this project
		var pages []*PagePlan
		for _, p := range plan.Pages {
			if strings.HasPrefix(p.TreePath, proj.TreePath+"/") {
				pages = append(pages, p)
			}
		}

		for _, page := range pages {
			if err := migratePage(repo, page, plan, progress); err != nil {
				progress.Fail(page.OldID, err)
				return fmt.Errorf("migrate page %s: %w", page.OldID, err)
			}
		}
	}

	slog.Info("git migration complete",
		"total", progress.Total,
		"migrated", progress.Done-progress.Skipped,
		"skipped", progress.Skipped,
		"failed", progress.Failed)

	// Phase 2: Create repo symlinks
	if err := CreateRepoSymlinks(plan); err != nil {
		return fmt.Errorf("create repo symlinks: %w", err)
	}

	// Phase 3: Create tree directory structure
	if err := createTree(plan); err != nil {
		return fmt.Errorf("create tree: %w", err)
	}

	return nil
}

// migratePage handles a single page: rename in git, update frontmatter, update cross-refs.
func migratePage(repo *git.Repo, page *PagePlan, plan *Plan, progress *Progress) error {
	// Check if already migrated (idempotent)
	if _, err := repo.ReadPage(page.Branch, page.NewPath); err == nil {
		progress.Skip(page.OldID)
		return nil
	}

	// Check old path exists
	content, err := repo.ReadPage(page.Branch, page.OldPath)
	if err != nil {
		return fmt.Errorf("read old path %s: %w", page.OldPath, err)
	}

	// Parse frontmatter
	fm, body, err := schema.Parse(content)
	if err != nil {
		return fmt.Errorf("parse frontmatter: %w", err)
	}

	// Update frontmatter
	fm.ID = page.UUID
	if !page.FirstCommit.IsZero() {
		fm.DCCreated = schema.Date{Time: page.FirstCommit}
	}

	// Update cross-references in body
	updatedBody := updateCrossRefs(string(body), plan)

	// Write the page at the new path with updated content
	msg := fmt.Sprintf("migrate: %s → %s (%s)", page.OldID, page.UUID, page.Title)
	if _, err := repo.WritePageWithMeta(page.Branch, page.NewPath, fm, []byte(updatedBody), msg, "migration-tool"); err != nil {
		return fmt.Errorf("write new path: %w", err)
	}

	// Delete the old path
	// git log --follow on the new path will trace through if content similarity >50%
	msgDel := fmt.Sprintf("migrate: remove old path %s", page.OldPath)
	if err := repo.DeletePage(page.Branch, page.OldPath, msgDel, "migration-tool"); err != nil {
		slog.Warn("could not delete old path (non-fatal)", "path", page.OldPath, "error", err)
	}

	progress.Log(page.OldID, page.Title)
	return nil
}

// updateCrossRefs replaces old-ID references in markdown with tree-path URLs.
func updateCrossRefs(body string, plan *Plan) string {
	result := body

	for oldID, page := range plan.Pages {
		// /projects/{any-project}/pages/{old-id} → /{tree-path}
		for projName := range plan.Projects {
			oldURL := fmt.Sprintf("/projects/%s/pages/%s", projName, oldID)
			newURL := "/" + page.TreePath
			result = strings.ReplaceAll(result, oldURL, newURL)
		}
	}

	return result
}

// createTree builds the tree/ directory with .uuid and .link files using package tree.
func createTree(plan *Plan) error {
	treeDir := plan.TreeDir

	for _, proj := range plan.Projects {
		if err := tree.WriteProjectUUIDFile(treeDir, proj.TreePath, proj.UUID, proj.DisplayName); err != nil {
			return fmt.Errorf("write project .uuid %s: %w", proj.TreePath, err)
		}
		slog.Info("created .uuid", "project", proj.TreePath)
	}

	byProjSlug := make(map[string]map[string]bool)
	for _, page := range plan.Pages {
		proj := plan.ProjectForTreePath(page.TreePath)
		if proj == nil {
			return fmt.Errorf("no project owns tree path %q", page.TreePath)
		}
		used := byProjSlug[proj.TreePath]
		if used == nil {
			used = make(map[string]bool)
			byProjSlug[proj.TreePath] = used
		}
		baseSlug := strings.TrimPrefix(page.TreePath, proj.TreePath+"/")
		slug := tree.UniqueSlug(baseSlug, used)
		if err := tree.WriteLinkFile(treeDir, proj.TreePath, slug, page.UUID, page.Title); err != nil {
			return fmt.Errorf("write .link for page %s: %w", page.OldID, err)
		}
	}

	slog.Info("tree created", "pages", len(plan.Pages), "projects", len(plan.Projects))
	return nil
}

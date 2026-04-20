package git

import (
	"fmt"
	"io"
	"sort"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// CommitInfo holds metadata about a single commit.
type CommitInfo struct {
	Hash    string    `json:"hash"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
	Message string    `json:"message"`
}

func mergeFetchLimit(limit, nPaths int) int {
	if nPaths <= 1 {
		return limit
	}
	n := limit * nPaths
	if n < 100 {
		n = 100
	}
	if n > 500 {
		n = 500
	}
	return n
}

func mergeCommitsDedupe(limit int, lists ...[]CommitInfo) []CommitInfo {
	if len(lists) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var all []CommitInfo
	for _, list := range lists {
		for _, c := range list {
			if seen[c.Hash] {
				continue
			}
			seen[c.Hash] = true
			all = append(all, c)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].Date.After(all[j].Date)
	})
	if len(all) > limit {
		all = all[:limit]
	}
	return all
}

// pageHistoryBranchPath returns commits touching path on a single branch (most recent first).
func (r *Repo) pageHistoryBranchPath(branch, path string, limit int) ([]CommitInfo, error) {
	if limit <= 0 {
		limit = 50
	}

	refName := plumbing.NewBranchReferenceName(branch)
	ref, err := r.repo.Reference(refName, true)
	if err != nil {
		return nil, fmt.Errorf("branch %q not found: %w", branch, err)
	}

	p := path
	logOpts := &gogit.LogOptions{
		From:     ref.Hash(),
		FileName: &p,
		Order:    gogit.LogOrderCommitterTime,
	}

	iter, err := r.repo.Log(logOpts)
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	defer iter.Close()

	var commits []CommitInfo
	err = iter.ForEach(func(c *object.Commit) error {
		if len(commits) >= limit {
			return fmt.Errorf("limit reached")
		}
		commits = append(commits, CommitInfo{
			Hash:    c.Hash.String(),
			Author:  c.Author.Name,
			Date:    c.Author.When,
			Message: c.Message,
		})
		return nil
	})
	if err != nil && err.Error() != "limit reached" {
		return nil, fmt.Errorf("walk log: %w", err)
	}

	return commits, nil
}

// PageHistory returns the commit history for a specific file on a branch.
// Results are most recent first. If limit <= 0, defaults to 50.
// aliasPaths are additional paths for the same logical page (e.g. pre-migration paths); history is merged and deduplicated by commit hash.
func (r *Repo) PageHistory(branch, path string, limit int, aliasPaths ...string) ([]CommitInfo, error) {
	if limit <= 0 {
		limit = 50
	}
	paths := append([]string{path}, aliasPaths...)
	fetch := mergeFetchLimit(limit, len(paths))
	var parts [][]CommitInfo
	for _, p := range paths {
		c, err := r.pageHistoryBranchPath(branch, p, fetch)
		if err != nil {
			continue
		}
		parts = append(parts, c)
	}
	if len(parts) == 0 {
		return []CommitInfo{}, nil
	}
	return mergeCommitsDedupe(limit, parts...), nil
}

// PageHistoryAllBranches returns commit history for a file across ALL branches,
// deduplicated by hash, sorted most recent first.
// aliasPaths are additional paths for the same logical page (e.g. pre-migration paths from migration-map.json).
func (r *Repo) PageHistoryAllBranches(path string, limit int, aliasPaths ...string) ([]CommitInfo, error) {
	if limit <= 0 {
		limit = 50
	}

	paths := append([]string{path}, aliasPaths...)
	fetch := mergeFetchLimit(limit, len(paths))

	branches, err := r.ListBranches()
	if err != nil {
		return nil, err
	}

	var parts [][]CommitInfo
	for _, branch := range branches {
		for _, p := range paths {
			commits, err := r.pageHistoryBranchPath(branch, p, fetch)
			if err != nil {
				continue
			}
			if len(commits) > 0 {
				parts = append(parts, commits)
			}
		}
	}
	if len(parts) == 0 {
		return []CommitInfo{}, nil
	}
	return mergeCommitsDedupe(limit, parts...), nil
}

// ReadPageAtCommitFirst tries paths in order and returns content from the first path that exists at the commit.
func (r *Repo) ReadPageAtCommitFirst(commitHash string, paths []string) ([]byte, string, error) {
	var lastErr error
	for _, p := range paths {
		data, err := r.ReadPageAtCommit(commitHash, p)
		if err == nil {
			return data, p, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, "", fmt.Errorf("file not found at commit %s in any of %d path(s): %w", shortHash(commitHash), len(paths), lastErr)
	}
	return nil, "", fmt.Errorf("no paths to try")
}

func shortHash(h string) string {
	if len(h) <= 8 {
		return h
	}
	return h[:8]
}

// FirstCommitDate returns the author date of the earliest commit that touched path on any branch.
func (r *Repo) FirstCommitDate(path string) (time.Time, error) {
	branches, err := r.ListBranches()
	if err != nil {
		return time.Time{}, err
	}
	var oldest time.Time
	found := false
	seen := make(map[string]bool)
	for _, branch := range branches {
		refName := plumbing.NewBranchReferenceName(branch)
		ref, err := r.repo.Reference(refName, true)
		if err != nil {
			continue
		}
		p := path
		logOpts := &gogit.LogOptions{
			From:     ref.Hash(),
			FileName: &p,
			Order:    gogit.LogOrderCommitterTime,
		}
		iter, err := r.repo.Log(logOpts)
		if err != nil {
			continue
		}
		ferr := iter.ForEach(func(c *object.Commit) error {
			h := c.Hash.String()
			if seen[h] {
				return nil
			}
			seen[h] = true
			d := c.Author.When
			if !found || d.Before(oldest) {
				oldest = d
				found = true
			}
			return nil
		})
		iter.Close()
		if ferr != nil {
			return time.Time{}, ferr
		}
	}
	if !found {
		return time.Time{}, fmt.Errorf("no commits for %q", path)
	}
	return oldest, nil
}

// ReadPageAtCommit reads a file's content at a specific commit hash.
func (r *Repo) ReadPageAtCommit(commitHash, path string) ([]byte, error) {
	h := plumbing.NewHash(commitHash)
	commit, err := r.repo.CommitObject(h)
	if err != nil {
		return nil, fmt.Errorf("commit %q not found: %w", commitHash, err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("tree: %w", err)
	}

	f, err := tree.File(path)
	if err != nil {
		return nil, fmt.Errorf("file %q not found at commit %s: %w", path, commitHash[:8], err)
	}

	reader, err := f.Reader()
	if err != nil {
		return nil, fmt.Errorf("reader: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return data, nil
}

// FindBranchForPage returns the first branch that contains the given path.
// Checks "truth" first, then other branches.
func (r *Repo) FindBranchForPage(path string) (string, error) {
	branches, err := r.ListBranches()
	if err != nil {
		return "", fmt.Errorf("list branches: %w", err)
	}

	// Prioritize truth, then drafts, then others.
	priority := []string{"truth", "draft/incoming", "draft/agent", "draft/test"}
	ordered := make([]string, 0, len(branches))
	for _, p := range priority {
		for _, b := range branches {
			if b == p {
				ordered = append(ordered, b)
				break
			}
		}
	}
	for _, b := range branches {
		found := false
		for _, o := range ordered {
			if b == o {
				found = true
				break
			}
		}
		if !found {
			ordered = append(ordered, b)
		}
	}

	for _, branch := range ordered {
		_, err := r.ReadPage(branch, path)
		if err == nil {
			return branch, nil
		}
	}
	return "", fmt.Errorf("page %q not found on any branch", path)
}

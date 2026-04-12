package git

import (
	"fmt"
	"io"
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

// PageHistory returns the commit history for a specific file on a branch.
// Results are most recent first. If limit <= 0, defaults to 50.
func (r *Repo) PageHistory(branch, path string, limit int) ([]CommitInfo, error) {
	if limit <= 0 {
		limit = 50
	}

	refName := plumbing.NewBranchReferenceName(branch)
	ref, err := r.repo.Reference(refName, true)
	if err != nil {
		return nil, fmt.Errorf("branch %q not found: %w", branch, err)
	}

	logOpts := &gogit.LogOptions{
		From:     ref.Hash(),
		FileName: &path,
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
	// "limit reached" is our own sentinel, not a real error.
	if err != nil && err.Error() != "limit reached" {
		return nil, fmt.Errorf("walk log: %w", err)
	}

	return commits, nil
}

// PageHistoryAllBranches returns commit history for a file across ALL branches,
// deduplicated by hash, sorted most recent first.
func (r *Repo) PageHistoryAllBranches(path string, limit int) ([]CommitInfo, error) {
	if limit <= 0 {
		limit = 50
	}

	branches, err := r.ListBranches()
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var all []CommitInfo

	for _, branch := range branches {
		commits, err := r.PageHistory(branch, path, limit)
		if err != nil {
			continue // branch may not have this file
		}
		for _, c := range commits {
			if !seen[c.Hash] {
				seen[c.Hash] = true
				all = append(all, c)
			}
		}
	}

	// Sort by date descending
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].Date.After(all[i].Date) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	if len(all) > limit {
		all = all[:limit]
	}

	return all, nil
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
		logOpts := &gogit.LogOptions{
			From:     ref.Hash(),
			FileName: &path,
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

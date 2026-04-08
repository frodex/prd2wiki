package git

import (
	"fmt"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/frodex/prd2wiki/internal/schema"
)

// WritePageWithMeta serializes frontmatter + body and writes to git.
func (r *Repo) WritePageWithMeta(branch, path string, fm *schema.Frontmatter, body []byte, message, author string) error {
	data, err := schema.Serialize(fm, body)
	if err != nil {
		return fmt.Errorf("serialize page: %w", err)
	}
	return r.WritePage(branch, path, data, message, author)
}

// ReadPageWithMeta reads a page and parses its frontmatter.
func (r *Repo) ReadPageWithMeta(branch, path string) (*schema.Frontmatter, []byte, error) {
	data, err := r.ReadPage(branch, path)
	if err != nil {
		return nil, nil, err
	}
	fm, body, err := schema.Parse(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parse page: %w", err)
	}
	return fm, body, nil
}

// DeletePage removes a file from a branch by committing a tree without it.
func (r *Repo) DeletePage(branch, path string, message, author string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	refName := plumbing.NewBranchReferenceName(branch)
	sig := &object.Signature{
		Name:  author,
		Email: author + "@prd2wiki",
		When:  time.Now(),
	}

	// Branch must exist to delete from it.
	ref, err := r.repo.Reference(refName, true)
	if err != nil {
		return fmt.Errorf("branch %q not found: %w", branch, err)
	}

	parentCommit, err := r.repo.CommitObject(ref.Hash())
	if err != nil {
		return fmt.Errorf("parent commit: %w", err)
	}

	tree, err := parentCommit.Tree()
	if err != nil {
		return fmt.Errorf("parent tree: %w", err)
	}

	// Collect all files except the one being deleted.
	files := make(map[string]plumbing.Hash)
	found := false
	if err := tree.Files().ForEach(func(f *object.File) error {
		if f.Name == path {
			found = true
			return nil // skip this file
		}
		files[f.Name] = f.Hash
		return nil
	}); err != nil {
		return fmt.Errorf("walk tree: %w", err)
	}

	if !found {
		return fmt.Errorf("file %q not found on branch %q", path, branch)
	}

	// Build a new tree without the deleted file.
	rootTreeHash, err := r.buildTree(files)
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}

	// Create commit.
	commit := &object.Commit{
		Author:       *sig,
		Committer:    *sig,
		Message:      message,
		TreeHash:     rootTreeHash,
		ParentHashes: []plumbing.Hash{parentCommit.Hash},
	}
	commitObj := &plumbing.MemoryObject{}
	if err := commit.Encode(commitObj); err != nil {
		return fmt.Errorf("encode commit: %w", err)
	}
	commitHash, err := r.repo.Storer.SetEncodedObject(commitObj)
	if err != nil {
		return fmt.Errorf("store commit: %w", err)
	}

	// Update branch reference.
	newRef := plumbing.NewHashReference(refName, commitHash)
	return r.repo.Storer.SetReference(newRef)
}

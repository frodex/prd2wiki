package git

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	billyos "github.com/go-git/go-billy/v5/osfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// Repo wraps a go-git bare repository with a mutex for serialized writes.
type Repo struct {
	repo *gogit.Repository
	path string
	mu   sync.Mutex
}

// repoPath returns the filesystem path for a project's bare repo.
func repoPath(dataDir, project string) string {
	return filepath.Join(dataDir, project+".wiki.git")
}

// InitRepo creates a new bare git repo at {dataDir}/{project}.wiki.git.
func InitRepo(dataDir, project string) (*Repo, error) {
	p := repoPath(dataDir, project)
	if err := os.MkdirAll(p, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", p, err)
	}
	dot := billyos.New(p)
	stor := filesystem.NewStorage(dot, cache.NewObjectLRUDefault())
	r, err := gogit.Init(stor, nil)
	if err != nil {
		return nil, fmt.Errorf("git init %s: %w", p, err)
	}
	return &Repo{repo: r, path: p}, nil
}

// OpenRepo opens an existing bare repo at {dataDir}/{project}.wiki.git.
func OpenRepo(dataDir, project string) (*Repo, error) {
	p := repoPath(dataDir, project)
	dot := billyos.New(p)
	stor := filesystem.NewStorage(dot, cache.NewObjectLRUDefault())
	r, err := gogit.Open(stor, nil)
	if err != nil {
		return nil, fmt.Errorf("git open %s: %w", p, err)
	}
	return &Repo{repo: r, path: p}, nil
}

// WritePage writes a file to a branch, creating the branch if needed.
// It serializes all writes with a mutex. Nested paths (e.g. "pages/test.md")
// are handled by building nested tree objects.
func (r *Repo) WritePage(branch, path string, content []byte, message, author string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	refName := plumbing.NewBranchReferenceName(branch)
	sig := &object.Signature{
		Name:  author,
		Email: author + "@prd2wiki",
		When:  time.Now(),
	}

	// Store the new blob.
	blobHash, err := r.storeBlob(content)
	if err != nil {
		return err
	}

	// Collect existing files from the branch (if it exists).
	files := make(map[string]plumbing.Hash)
	var parentCommit *object.Commit

	ref, err := r.repo.Reference(refName, true)
	if err == nil {
		parentCommit, err = r.repo.CommitObject(ref.Hash())
		if err != nil {
			return fmt.Errorf("parent commit: %w", err)
		}
		tree, err := parentCommit.Tree()
		if err != nil {
			return fmt.Errorf("parent tree: %w", err)
		}
		if err := tree.Files().ForEach(func(f *object.File) error {
			files[f.Name] = f.Hash
			return nil
		}); err != nil {
			return fmt.Errorf("walk tree: %w", err)
		}
	}

	// Update the file map.
	files[path] = blobHash

	// Build nested tree objects.
	rootTreeHash, err := r.buildTree(files)
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}

	// Create commit.
	commit := &object.Commit{
		Author:    *sig,
		Committer: *sig,
		Message:   message,
		TreeHash:  rootTreeHash,
	}
	if parentCommit != nil {
		commit.ParentHashes = []plumbing.Hash{parentCommit.Hash}
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

// WritePageWithDate writes a page with a specific author date (for backfilling).
func (r *Repo) WritePageWithDate(branch, path string, content []byte, message, author string, date time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	refName := plumbing.NewBranchReferenceName(branch)
	sig := &object.Signature{
		Name:  author,
		Email: author,
		When:  date,
	}

	blobHash, err := r.storeBlob(content)
	if err != nil {
		return err
	}

	files := make(map[string]plumbing.Hash)
	var parentCommit *object.Commit

	ref, err := r.repo.Reference(refName, true)
	if err == nil {
		parentCommit, err = r.repo.CommitObject(ref.Hash())
		if err != nil {
			return fmt.Errorf("parent commit: %w", err)
		}
		tree, err := parentCommit.Tree()
		if err != nil {
			return fmt.Errorf("parent tree: %w", err)
		}
		if err := tree.Files().ForEach(func(f *object.File) error {
			files[f.Name] = f.Hash
			return nil
		}); err != nil {
			return fmt.Errorf("walk tree: %w", err)
		}
	}

	files[path] = blobHash

	rootTreeHash, err := r.buildTree(files)
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}

	commit := &object.Commit{
		Author:    *sig,
		Committer: *sig,
		Message:   message,
		TreeHash:  rootTreeHash,
	}
	if parentCommit != nil {
		commit.ParentHashes = []plumbing.Hash{parentCommit.Hash}
	}
	commitObj := &plumbing.MemoryObject{}
	if err := commit.Encode(commitObj); err != nil {
		return fmt.Errorf("encode commit: %w", err)
	}
	commitHash, err := r.repo.Storer.SetEncodedObject(commitObj)
	if err != nil {
		return fmt.Errorf("store commit: %w", err)
	}

	newRef := plumbing.NewHashReference(refName, commitHash)
	return r.repo.Storer.SetReference(newRef)
}

// ReadPage reads a file from a branch.
func (r *Repo) ReadPage(branch, path string) ([]byte, error) {
	refName := plumbing.NewBranchReferenceName(branch)
	ref, err := r.repo.Reference(refName, true)
	if err != nil {
		return nil, fmt.Errorf("branch %q not found: %w", branch, err)
	}
	commit, err := r.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("tree: %w", err)
	}
	f, err := tree.File(path)
	if err != nil {
		return nil, fmt.Errorf("file %q not found: %w", path, err)
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

// ListBranches returns all branch names.
func (r *Repo) ListBranches() ([]string, error) {
	refs, err := r.repo.References()
	if err != nil {
		return nil, fmt.Errorf("references: %w", err)
	}
	var branches []string
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsBranch() {
			branches = append(branches, ref.Name().Short())
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterate refs: %w", err)
	}
	return branches, nil
}

// ListPages returns all file paths on a branch.
func (r *Repo) ListPages(branch string) ([]string, error) {
	refName := plumbing.NewBranchReferenceName(branch)
	ref, err := r.repo.Reference(refName, true)
	if err != nil {
		return nil, fmt.Errorf("branch %q not found: %w", branch, err)
	}
	commit, err := r.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("tree: %w", err)
	}
	var paths []string
	err = tree.Files().ForEach(func(f *object.File) error {
		paths = append(paths, f.Name)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk: %w", err)
	}
	return paths, nil
}

// storeBlob writes content as a blob object and returns its hash.
func (r *Repo) storeBlob(content []byte) (plumbing.Hash, error) {
	obj := &plumbing.MemoryObject{}
	obj.SetType(plumbing.BlobObject)
	obj.SetSize(int64(len(content)))
	w, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("blob writer: %w", err)
	}
	if _, err := w.Write(content); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("blob write: %w", err)
	}
	if err := w.Close(); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("blob close: %w", err)
	}
	h, err := r.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("store blob: %w", err)
	}
	return h, nil
}

// dirNode represents a directory in the tree being built.
type dirNode struct {
	files   map[string]plumbing.Hash // filename -> blob hash
	subdirs map[string]*dirNode      // dirname -> subtree
}

// buildTree constructs nested tree objects from a flat map of path -> blob hash.
func (r *Repo) buildTree(files map[string]plumbing.Hash) (plumbing.Hash, error) {
	root := &dirNode{
		files:   make(map[string]plumbing.Hash),
		subdirs: make(map[string]*dirNode),
	}

	for path, hash := range files {
		parts := strings.Split(path, "/")
		node := root
		for i, part := range parts {
			if i == len(parts)-1 {
				node.files[part] = hash
			} else {
				sub, ok := node.subdirs[part]
				if !ok {
					sub = &dirNode{
						files:   make(map[string]plumbing.Hash),
						subdirs: make(map[string]*dirNode),
					}
					node.subdirs[part] = sub
				}
				node = sub
			}
		}
	}

	return r.storeTreeNode(root)
}

// storeTreeNode recursively stores a dirNode as git tree objects (bottom-up).
func (r *Repo) storeTreeNode(node *dirNode) (plumbing.Hash, error) {
	var entries []object.TreeEntry

	// Add file entries.
	for name, hash := range node.files {
		entries = append(entries, object.TreeEntry{
			Name: name,
			Mode: filemode.Regular,
			Hash: hash,
		})
	}

	// Add subdirectory entries (recurse first to get their hashes).
	for name, sub := range node.subdirs {
		subHash, err := r.storeTreeNode(sub)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		entries = append(entries, object.TreeEntry{
			Name: name,
			Mode: filemode.Dir,
			Hash: subHash,
		})
	}

	// Sort entries by name (git requires sorted tree entries).
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	tree := &object.Tree{Entries: entries}
	treeObj := &plumbing.MemoryObject{}
	if err := tree.Encode(treeObj); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("encode tree: %w", err)
	}
	h, err := r.repo.Storer.SetEncodedObject(treeObj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("store tree: %w", err)
	}
	return h, nil
}

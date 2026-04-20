package tree

import (
	"fmt"
	"strings"
	"sync"
)

// IndexHolder holds the scanned tree index and refreshes it after CRUD on .link files.
type IndexHolder struct {
	mu       sync.RWMutex
	idx      *Index
	treeRoot string
	dataDir  string
}

// NewIndexHolder creates a holder; initial must be non-nil for normal operation.
func NewIndexHolder(treeRoot, dataDir string, initial *Index) *IndexHolder {
	return &IndexHolder{idx: initial, treeRoot: treeRoot, dataDir: dataDir}
}

// Get returns the current tree index (may be nil only if misconfigured).
func (h *IndexHolder) Get() *Index {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.idx
}

// Refresh rescans the tree directory and replaces the in-memory index.
func (h *IndexHolder) Refresh() error {
	if h == nil {
		return fmt.Errorf("tree: nil IndexHolder")
	}
	idx, err := Scan(h.treeRoot, h.dataDir)
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.idx = idx
	h.mu.Unlock()
	return nil
}

// TreeRoot returns the absolute tree root path.
func (h *IndexHolder) TreeRoot() string {
	if h == nil {
		return ""
	}
	return h.treeRoot
}

// UpdateLibrarianHeadInLink writes librarian head ID to .link line 2 and rescans the tree index.
func (h *IndexHolder) UpdateLibrarianHeadInLink(pageUUID, librarianHeadID string) error {
	if h == nil {
		return fmt.Errorf("tree: nil IndexHolder")
	}
	idx := h.Get()
	if idx == nil {
		return fmt.Errorf("tree: nil index")
	}
	ent, ok := idx.PageByUUID(strings.TrimSpace(pageUUID))
	if !ok {
		return fmt.Errorf("tree: unknown page UUID %s", pageUUID)
	}
	if err := UpdateLinkFileLibrarianHead(h.treeRoot, ent.Page.TreePath, pageUUID, librarianHeadID); err != nil {
		return err
	}
	return h.Refresh()
}

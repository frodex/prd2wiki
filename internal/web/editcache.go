package web

import (
	"sync"
	"time"

	wgit "github.com/frodex/prd2wiki/internal/git"
)

// EditInfo caches last-edit metadata for a page path.
type EditInfo struct {
	Author string
	Date   string // formatted "2006-01-02 15:04"
}

// EditCache stores last-edit info per page path, built once from git on startup.
// Updated incrementally on writes. Reads are lock-free after initial build.
// Shared between web handler and API server.
type EditCache struct {
	mu    sync.RWMutex
	items map[string]EditInfo // key: page path (e.g. "pages/04/20776.md")
}

func NewEditCache() *EditCache {
	return &EditCache{items: make(map[string]EditInfo)}
}

// build populates the cache from git history for all pages.
// Called once at startup. Walks git log once for the entire repo.
func (c *EditCache) Build(repo *wgit.Repo, paths []string) {
	if repo == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, path := range paths {
		commits, err := repo.PageHistoryAllBranches(path, 1)
		if err != nil || len(commits) == 0 {
			continue
		}
		c.items[path] = EditInfo{
			Author: commits[0].Author,
			Date:   commits[0].Date.Format("2006-01-02 15:04"),
		}
	}
}

// get returns cached edit info for a page path.
func (c *EditCache) Get(path string) (EditInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	info, ok := c.items[path]
	return info, ok
}

// touch updates the cache for a page that was just written.
func (c *EditCache) Touch(path, author string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[path] = EditInfo{Author: author, Date: time.Now().UTC().Format("2006-01-02 15:04")}
}

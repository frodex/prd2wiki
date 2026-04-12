package web

import (
	"strings"
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

const editCacheHistoryLimit = 100

// build populates the cache from git history for all pages.
// Called once at startup. Walks git log once for the entire repo.
// Commits whose first line starts with "migrate:" are skipped so post-migration
// metadata reflects the last real edit, not the migration tooling.
// migrationAliases maps current git paths to pre-migration paths for combined history.
func (c *EditCache) Build(repo *wgit.Repo, paths []string, migrationAliases map[string][]string) {
	if repo == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, path := range paths {
		extras := migrationExtraPaths(path, migrationAliases)
		commits, err := repo.PageHistoryAllBranches(path, editCacheHistoryLimit, extras...)
		if err != nil || len(commits) == 0 {
			continue
		}
		chosen := pickLastNonMigrateCommit(commits)
		c.items[path] = EditInfo{
			Author: chosen.Author,
			Date:   chosen.Date.Format("2006-01-02 15:04"),
		}
	}
}

// pickLastNonMigrateCommit returns the newest commit that is not a migration commit.
// History is newest-first. If every commit is a migration commit, returns the newest (defensive).
func pickLastNonMigrateCommit(commits []wgit.CommitInfo) wgit.CommitInfo {
	for _, c := range commits {
		if !isMigrateCommitMessage(c.Message) {
			return c
		}
	}
	return commits[0]
}

func isMigrateCommitMessage(message string) bool {
	first := strings.TrimSpace(message)
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = strings.TrimSpace(first[:i])
	}
	return strings.HasPrefix(first, "migrate:")
}

func migrationExtraPaths(path string, m map[string][]string) []string {
	if m == nil {
		return nil
	}
	return m[path]
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

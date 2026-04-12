// Package tree maps the on-disk wiki tree (.uuid / .link) to git projects and pages.
package tree

import (
	"path/filepath"
	"strings"
)

// Project is one directory under the tree root that contains a .uuid file.
type Project struct {
	UUID    string // full project UUID (line 1 of .uuid)
	Name    string // display name (line 2 of .uuid)
	Path    string // relative path under tree root using "/" (e.g. "prd2wiki", "games/battletech")
	RepoPath string // absolute path to the bare git repo: data/repos/proj_{uuid[:8]}.git
	// RepoKey is the wiki project key used with git.OpenRepo(dataDir, key), e.g. "default".
	RepoKey string
}

// RepoPathForUUID returns the conventional bare-repo path for a project UUID
// (proj_{first segment before hyphen}.git, same rule as Pre-flight B).
func RepoPathForUUID(dataDir, uuid string) string {
	pre := strings.SplitN(strings.TrimSpace(uuid), "-", 2)[0]
	if pre == "" {
		return ""
	}
	return filepath.Join(dataDir, "repos", "proj_"+pre+".git")
}

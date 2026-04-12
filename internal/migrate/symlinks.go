package migrate

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// CreateRepoSymlinks creates data/repos/proj_{uuid8}.git symlinks pointing
// to the original {name}.wiki.git repos. The tree scanner expects this layout.
func CreateRepoSymlinks(plan *Plan) error {
	reposDir := filepath.Join(plan.DataDir, "repos")
	if err := os.MkdirAll(reposDir, 0755); err != nil {
		return fmt.Errorf("mkdir repos: %w", err)
	}

	for _, proj := range plan.Projects {
		if len(proj.UUID) < 8 {
			return fmt.Errorf("project %s UUID too short: %q", proj.OldName, proj.UUID)
		}
		linkName := fmt.Sprintf("proj_%s.git", proj.UUID[:8])
		linkPath := filepath.Join(reposDir, linkName)

		// Target is relative: ../default.wiki.git
		target := fmt.Sprintf("../%s.wiki.git", proj.OldName)

		// Remove existing symlink if present
		if _, err := os.Lstat(linkPath); err == nil {
			os.Remove(linkPath)
		}

		if err := os.Symlink(target, linkPath); err != nil {
			return fmt.Errorf("symlink %s → %s: %w", linkPath, target, err)
		}
		slog.Info("created repo symlink", "link", linkPath, "target", target)
	}
	return nil
}

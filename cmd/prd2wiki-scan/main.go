// prd2wiki-scan walks a project directory and outputs a YAML proposal
// describing which markdown files to ingest into the wiki.
//
// Usage: prd2wiki-scan /srv/battletech > battletech-scan.yaml
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ScanResult is the top-level YAML output.
type ScanResult struct {
	Project ProjectInfo `yaml:"project"`
	Pages   []PageEntry `yaml:"pages"`
	Skipped []string    `yaml:"skipped,omitempty"`
	Summary string      `yaml:"summary"`
}

// ProjectInfo describes the source project.
type ProjectInfo struct {
	ID        string `yaml:"id"`
	Title     string `yaml:"title"`
	SourceDir string `yaml:"source_dir"`
	GitRemote string `yaml:"git_remote,omitempty"`
}

// PageEntry describes one markdown file to ingest.
type PageEntry struct {
	File     string   `yaml:"file"`     // relative path from project root
	ID       string   `yaml:"id"`       // suggested wiki page ID (slug)
	Title    string   `yaml:"title"`    // from first heading or filename
	Type     string   `yaml:"type"`     // concept, task, requirement, reference, config, project
	Module   string   `yaml:"module"`   // docs, config, agents, etc.
	Category string   `yaml:"category"` // research, plans, specs, etc.
	Tags     []string `yaml:"tags"`
}

// Directories to skip while walking.
var skipDirs = map[string]bool{
	".git":         true,
	".cursor":      true,
	"node_modules": true,
	"vendor":       true,
	"bin":          true,
	"data":         true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: prd2wiki-scan <project-dir>")
		os.Exit(1)
	}
	root, err := filepath.Abs(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad path: %v\n", err)
		os.Exit(1)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "%s is not a directory\n", root)
		os.Exit(1)
	}

	projectName := filepath.Base(root)
	result := ScanResult{
		Project: ProjectInfo{
			ID:        slugify(projectName),
			Title:     projectName,
			SourceDir: root,
			GitRemote: gitRemote(root),
		},
	}

	err = filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if fi.IsDir() {
			if skipDirs[fi.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(fi.Name(), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		entry := classify(rel, projectName, path)
		result.Pages = append(result.Pages, entry)
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "walk error: %v\n", err)
		os.Exit(1)
	}

	// Sort pages by file path for deterministic output.
	sort.Slice(result.Pages, func(i, j int) bool {
		return result.Pages[i].File < result.Pages[j].File
	})

	result.Summary = fmt.Sprintf("Found %d markdown files in %s", len(result.Pages), projectName)

	out, err := yaml.Marshal(&result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "yaml marshal: %v\n", err)
		os.Exit(1)
	}
	os.Stdout.Write(out)
}

// classify determines the page metadata based on its relative path.
func classify(rel, projectName, absPath string) PageEntry {
	entry := PageEntry{
		File:  rel,
		ID:    slugFromFilename(rel),
		Title: readFirstHeading(absPath, rel),
		Tags:  []string{projectName},
	}

	// Normalize path separators and lowercase for matching.
	norm := strings.ToLower(filepath.ToSlash(rel))
	parts := strings.Split(norm, "/")

	switch {
	// docs/research/*.md
	case matchPath(norm, "docs/research/") || matchPath(norm, "docs/superpowers/research/"):
		entry.Type = "concept"
		entry.Module = "docs"
		entry.Category = "research"

	// docs/superpowers/specs/*.md or docs/specs/*.md
	case matchPath(norm, "docs/superpowers/specs/") || matchPath(norm, "docs/specs/"):
		entry.Type = "requirement"
		entry.Module = "docs"
		entry.Category = "specs"

	// docs/superpowers/plans/*.md or docs/plans/*.md
	case matchPath(norm, "docs/superpowers/plans/") || matchPath(norm, "docs/plans/"):
		entry.Type = "task"
		entry.Module = "docs"
		entry.Category = "plans"

	// docs/solutions/*.md
	case matchPath(norm, "docs/solutions/"):
		entry.Type = "concept"
		entry.Module = "docs"
		entry.Category = "solutions"

	// docs/design/*.md
	case matchPath(norm, "docs/design/"):
		entry.Type = "concept"
		entry.Module = "docs"
		entry.Category = "design"

	// docs/integration/*.md
	case matchPath(norm, "docs/integration/"):
		entry.Type = "reference"
		entry.Module = "docs"
		entry.Category = "integration"

	// steward/*.md
	case matchPath(norm, "steward/"):
		entry.Type = "reference"
		entry.Module = "agents"

	// Root-level special files
	case len(parts) == 1:
		base := parts[0]
		switch {
		case base == "sessions.md":
			entry.Type = "config"
			entry.Module = "config"
		case base == "claude.md" || base == "readme.md":
			entry.Type = "reference"
			entry.Module = "config"
		default:
			entry.Type = "concept"
			entry.Module = "docs"
		}

	// Anything else under docs/
	case matchPath(norm, "docs/"):
		entry.Type = "reference"
		entry.Module = "docs"

	// Any *.md in subdirectories with CLAUDE.md
	case strings.HasSuffix(norm, "claude.md"):
		entry.Type = "reference"
		entry.Module = "config"

	// Fallback
	default:
		entry.Type = "concept"
		entry.Module = "docs"
	}

	return entry
}

// matchPath checks if the normalized path starts with the given prefix.
func matchPath(norm, prefix string) bool {
	return strings.HasPrefix(norm, prefix)
}

// slugFromFilename generates a wiki page ID from a relative file path.
// e.g. "docs/research/mechlab-research.md" -> "mechlab-research"
func slugFromFilename(rel string) string {
	base := filepath.Base(rel)
	base = strings.TrimSuffix(base, ".md")
	return slugify(base)
}

// slugify converts a string to a URL-safe slug.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}

// readFirstHeading reads the first markdown heading from a file.
// Falls back to the filename if no heading found.
func readFirstHeading(path, rel string) string {
	f, err := os.Open(path)
	if err != nil {
		return filenameTitle(rel)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "#"))
		}
	}
	return filenameTitle(rel)
}

// filenameTitle converts a filename to a title.
func filenameTitle(rel string) string {
	base := filepath.Base(rel)
	base = strings.TrimSuffix(base, ".md")
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.ReplaceAll(base, "_", " ")
	return base
}

// gitRemote tries to get the git remote URL for the project directory.
func gitRemote(dir string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

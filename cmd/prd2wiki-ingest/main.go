// prd2wiki-ingest reads a scan YAML plan and imports each page into the wiki.
//
// Usage: prd2wiki-ingest --plan battletech-scan.yaml [--data-dir ./data] [--dry-run]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/schema"
)

// ScanResult mirrors the scan tool's output.
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
	File     string   `yaml:"file"`
	ID       string   `yaml:"id"`
	Title    string   `yaml:"title"`
	Type     string   `yaml:"type"`
	Module   string   `yaml:"module"`
	Category string   `yaml:"category"`
	Tags     []string `yaml:"tags"`
}

func main() {
	planFile := flag.String("plan", "", "path to scan YAML plan file (required)")
	dataDir := flag.String("data-dir", "./data", "wiki data directory")
	project := flag.String("project", "", "wiki project name (default: from plan's project.id)")
	branch := flag.String("branch", "draft/incoming", "git branch to write to")
	dryRun := flag.Bool("dry-run", false, "show what would be ingested without doing it")
	flag.Parse()

	if *planFile == "" {
		fmt.Fprintln(os.Stderr, "error: --plan is required")
		flag.Usage()
		os.Exit(1)
	}

	// Read and parse the plan.
	data, err := os.ReadFile(*planFile)
	if err != nil {
		log.Fatalf("read plan: %v", err)
	}
	var plan ScanResult
	if err := yaml.Unmarshal(data, &plan); err != nil {
		log.Fatalf("parse plan: %v", err)
	}

	// Determine project name.
	proj := *project
	if proj == "" {
		proj = plan.Project.ID
	}
	if proj == "" {
		proj = "default"
	}

	if *dryRun {
		fmt.Printf("DRY RUN — would ingest %d pages into project %q (branch: %s)\n\n", len(plan.Pages), proj, *branch)
		for i, p := range plan.Pages {
			fmt.Printf("[%d/%d] %s → %s (type=%s, module=%s)\n", i+1, len(plan.Pages), p.File, p.ID, p.Type, p.Module)
		}
		fmt.Println("\nNo changes made.")
		return
	}

	// Open or create the wiki git repo.
	repo, err := openOrInitRepo(*dataDir, proj)
	if err != nil {
		log.Fatalf("open repo: %v", err)
	}

	total := len(plan.Pages)
	ingested := 0
	skipped := 0

	// Create the project page if none exists.
	projectPagePath := fmt.Sprintf("pages/PROJECT-%s.md", proj)
	if !repo.HasPage(*branch, projectPagePath) {
		if err := createProjectPage(repo, *branch, plan.Project, proj); err != nil {
			log.Printf("WARN: could not create project page: %v", err)
		} else {
			fmt.Printf("[0/%d] created project page → PROJECT-%s\n", total, proj)
		}
	}

	for i, p := range plan.Pages {
		srcPath := filepath.Join(plan.Project.SourceDir, p.File)

		body, err := os.ReadFile(srcPath)
		if err != nil {
			log.Printf("SKIP %s: %v", p.File, err)
			skipped++
			continue
		}

		// Get the file's modification time for backdating the commit.
		fi, err := os.Stat(srcPath)
		if err != nil {
			log.Printf("SKIP %s: stat: %v", p.File, err)
			skipped++
			continue
		}
		modTime := fi.ModTime()

		// Strip any existing frontmatter from the source file.
		_, rawBody, parseErr := schema.Parse(body)
		if parseErr != nil {
			// If parsing fails, use the whole file as body.
			rawBody = body
		}

		// Build frontmatter.
		fm := &schema.Frontmatter{
			ID:         p.ID,
			Title:      p.Title,
			Type:       p.Type,
			Status:     "draft",
			Tags:       p.Tags,
			Module:     p.Module,
			Category:   p.Category,
			ProjectRef: proj,
			DCCreator:  "scan-ingest@prd2wiki",
			DCCreated:  schema.Date{Time: modTime},
		}

		serialized, err := schema.Serialize(fm, rawBody)
		if err != nil {
			log.Printf("SKIP %s: serialize: %v", p.File, err)
			skipped++
			continue
		}

		pagePath := fmt.Sprintf("pages/%s.md", p.ID)
		msg := fmt.Sprintf("ingest: %s from %s", p.ID, p.File)

		err = repo.WritePageWithDate(*branch, pagePath, serialized, msg, "scan-ingest@prd2wiki", modTime)
		if err != nil {
			log.Printf("FAIL %s: %v", p.File, err)
			skipped++
			continue
		}

		ingested++
		fmt.Printf("[%d/%d] ingested %s → %s\n", i+1, total, p.File, p.ID)
	}

	fmt.Printf("\nDone. %d ingested, %d skipped out of %d total.\n", ingested, skipped, total)
}

// openOrInitRepo opens an existing repo or initializes a new one.
func openOrInitRepo(dataDir, project string) (*wgit.Repo, error) {
	repo, err := wgit.OpenRepo(dataDir, project)
	if err != nil {
		// Try to initialize a new repo.
		repo, err = wgit.InitRepo(dataDir, project)
		if err != nil {
			return nil, fmt.Errorf("init repo: %w", err)
		}
	}
	return repo, nil
}

// createProjectPage creates a type=project page summarizing the ingested project.
func createProjectPage(repo *wgit.Repo, branch string, info ProjectInfo, proj string) error {
	fm := &schema.Frontmatter{
		ID:        "PROJECT-" + proj,
		Title:     info.Title,
		Type:      "project",
		Status:    "active",
		Tags:      []string{proj, "project"},
		Module:    "config",
		DCCreator: "scan-ingest@prd2wiki",
		DCCreated: schema.Date{Time: time.Now()},
	}

	body := fmt.Sprintf("# %s\n\nProject ingested from `%s`.\n", info.Title, info.SourceDir)
	if info.GitRemote != "" {
		body += fmt.Sprintf("\nGit remote: %s\n", info.GitRemote)
	}

	serialized, err := schema.Serialize(fm, []byte(body))
	if err != nil {
		return err
	}

	pagePath := fmt.Sprintf("pages/PROJECT-%s.md", proj)
	return repo.WritePage(branch, pagePath, serialized, "ingest: create project page for "+proj, "scan-ingest@prd2wiki")
}

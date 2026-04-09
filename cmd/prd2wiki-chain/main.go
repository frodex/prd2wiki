// prd2wiki-chain detects version chains in scanned markdown files and
// optionally re-imports them with proper git history.
//
// Chain detection groups files like:
//   mechlab-research.md, mechlab-research-NOTES-01a.md, mechlab-research-revised-02.md
// into a single page with sequential commits showing the document's evolution.
//
// Usage:
//   prd2wiki-chain --scan /tmp/bt-scan.yaml                          # detect chains → stdout
//   prd2wiki-chain --ingest /tmp/bt-chains.yaml --data-dir ./data    # import chains
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/schema"
)

// --- Scan input types (mirrors prd2wiki-scan output) ---

type ScanResult struct {
	Project ProjectInfo `yaml:"project"`
	Pages   []PageEntry `yaml:"pages"`
	Skipped []string    `yaml:"skipped,omitempty"`
	Summary string      `yaml:"summary"`
}

type ProjectInfo struct {
	ID        string `yaml:"id"`
	Title     string `yaml:"title"`
	SourceDir string `yaml:"source_dir"`
	GitRemote string `yaml:"git_remote,omitempty"`
}

type PageEntry struct {
	File     string   `yaml:"file"`
	ID       string   `yaml:"id"`
	Title    string   `yaml:"title"`
	Type     string   `yaml:"type"`
	Module   string   `yaml:"module"`
	Category string   `yaml:"category"`
	Tags     []string `yaml:"tags"`
}

// --- Chain manifest types ---

type ChainManifest struct {
	Project    ManifestProject `yaml:"project"`
	Chains     []Chain         `yaml:"chains"`
	Standalone []StandalonePage `yaml:"standalone"`
}

type ManifestProject struct {
	ID        string `yaml:"id"`
	SourceDir string `yaml:"source_dir"`
}

type Chain struct {
	BaseID   string          `yaml:"base_id"`
	Versions []ChainVersion  `yaml:"versions"`
}

type ChainVersion struct {
	File    string `yaml:"file"`
	ID      string `yaml:"id"`
	Title   string `yaml:"title,omitempty"`
	Type    string `yaml:"type,omitempty"`
	Module  string `yaml:"module,omitempty"`
	Category string `yaml:"category,omitempty"`
	Tags    []string `yaml:"tags,omitempty"`
	Author  string `yaml:"author"`
	Date    string `yaml:"date"`
	Message string `yaml:"message"`
}

type StandalonePage struct {
	File     string   `yaml:"file"`
	ID       string   `yaml:"id"`
	Title    string   `yaml:"title"`
	Type     string   `yaml:"type"`
	Module   string   `yaml:"module"`
	Category string   `yaml:"category,omitempty"`
	Tags     []string `yaml:"tags"`
	Author   string   `yaml:"author"`
	Date     string   `yaml:"date"`
}

// --- Version suffix stripping ---

// versionPatterns are applied in order (most specific first) to strip
// version indicators from a filename base. The base is the filename
// without extension and without any leading date prefix.
var versionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`-revised-\d+$`),
	regexp.MustCompile(`-NOTES-\d+[a-z]?$`),
	regexp.MustCompile(`-NOTES-[a-z]+$`),
	regexp.MustCompile(`-NOTES$`),
	regexp.MustCompile(`-v\d+\.\d+$`),       // trailing: foo-v0.1
	regexp.MustCompile(`^v\d+\.\d+-`),        // leading: v0.1-foo (after date strip)
	regexp.MustCompile(`\.\d+\.\d+\.\d+\.backup$`), // foo.0.0.1.backup (before semver!)
	regexp.MustCompile(`\.\d+\.\d+\.\d+$`),  // trailing semver: foo.0.0.1
	regexp.MustCompile(`-\d{2,}$`), // trailing 2+ digit number (NOT single digit like plan1)
}

// stripVersionSuffix removes version indicators from a filename base.
// It applies patterns repeatedly until no more match, to handle stacked
// suffixes like "-03-NOTES".
func stripVersionSuffix(name string) string {
	for {
		changed := false
		for _, pat := range versionPatterns {
			stripped := pat.ReplaceAllString(name, "")
			if stripped != name {
				name = stripped
				changed = true
				break // restart from most specific
			}
		}
		if !changed {
			return name
		}
	}
}

// datePrefix matches leading YYYY-MM-DD- in filenames.
var datePrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-`)

// baseIDFromFile computes the chain grouping key from a file path.
// It strips the directory, extension, leading date prefix, and version suffixes.
func baseIDFromFile(file string) string {
	name := filepath.Base(file)
	name = strings.TrimSuffix(name, ".md")

	// Strip leading date prefix for grouping purposes.
	name = datePrefix.ReplaceAllString(name, "")

	base := stripVersionSuffix(name)
	// Clean up leading/trailing dashes from stripping
	base = strings.Trim(base, "-")
	return base
}

// detectAuthor returns "greg" for files with "NOTES" in the name, "claude" otherwise.
func detectAuthor(file string) string {
	if strings.Contains(strings.ToUpper(filepath.Base(file)), "NOTES") {
		return "greg"
	}
	return "claude"
}

// commitMessage generates a descriptive commit message for a chain version.
func commitMessage(file, baseID string) string {
	base := filepath.Base(file)
	base = strings.TrimSuffix(base, ".md")
	lower := strings.ToLower(base)

	switch {
	case strings.Contains(lower, "-notes-extracted"):
		return "notes: extracted semantic deltas"
	case strings.Contains(lower, "-notes-"):
		return fmt.Sprintf("notes: greg's review of %s", baseID)
	case strings.Contains(lower, "-notes"):
		return fmt.Sprintf("notes: greg's review of %s", baseID)
	case strings.Contains(lower, "-revised-"):
		// Extract the revision number
		re := regexp.MustCompile(`-revised-(\d+)`)
		m := re.FindStringSubmatch(lower)
		if len(m) > 1 {
			return fmt.Sprintf("research: revised-%s, incorporating review notes", m[1])
		}
		return fmt.Sprintf("research: revised, incorporating review notes")
	default:
		return fmt.Sprintf("research: initial %s", baseID)
	}
}

func main() {
	scanFile := flag.String("scan", "", "path to scan YAML (chain detection mode)")
	ingestFile := flag.String("ingest", "", "path to chain manifest YAML (ingest mode)")
	dataDir := flag.String("data-dir", "./data", "wiki data directory")
	branch := flag.String("branch", "draft/incoming", "git branch to write to")
	dryRun := flag.Bool("dry-run", false, "show what would be done without doing it")
	flag.Parse()

	if *scanFile == "" && *ingestFile == "" {
		fmt.Fprintln(os.Stderr, "error: --scan or --ingest is required")
		flag.Usage()
		os.Exit(1)
	}

	if *scanFile != "" {
		runDetect(*scanFile)
		return
	}

	runIngest(*ingestFile, *dataDir, *branch, *dryRun)
}

// --- Detection mode ---

func runDetect(scanFile string) {
	data, err := os.ReadFile(scanFile)
	if err != nil {
		log.Fatalf("read scan: %v", err)
	}
	var scan ScanResult
	if err := yaml.Unmarshal(data, &scan); err != nil {
		log.Fatalf("parse scan: %v", err)
	}

	// Group pages by base ID.
	groups := make(map[string][]PageEntry)
	order := []string{} // preserve first-seen order
	for _, p := range scan.Pages {
		base := baseIDFromFile(p.File)
		if _, exists := groups[base]; !exists {
			order = append(order, base)
		}
		groups[base] = append(groups[base], p)
	}

	// Sort each group by file modification time.
	for base, pages := range groups {
		sortByModTime(pages, scan.Project.SourceDir)
		groups[base] = pages
	}

	manifest := ChainManifest{
		Project: ManifestProject{
			ID:        scan.Project.ID,
			SourceDir: scan.Project.SourceDir,
		},
	}

	for _, base := range order {
		pages := groups[base]
		if len(pages) == 1 {
			p := pages[0]
			modTime := fileModTime(filepath.Join(scan.Project.SourceDir, p.File))
			manifest.Standalone = append(manifest.Standalone, StandalonePage{
				File:     p.File,
				ID:       p.ID,
				Title:    p.Title,
				Type:     p.Type,
				Module:   p.Module,
				Category: p.Category,
				Tags:     p.Tags,
				Author:   detectAuthor(p.File),
				Date:     modTime.Format(time.RFC3339),
			})
			continue
		}

		chain := Chain{BaseID: base}
		for _, p := range pages {
			modTime := fileModTime(filepath.Join(scan.Project.SourceDir, p.File))
			chain.Versions = append(chain.Versions, ChainVersion{
				File:     p.File,
				ID:       p.ID,
				Title:    p.Title,
				Type:     p.Type,
				Module:   p.Module,
				Category: p.Category,
				Tags:     p.Tags,
				Author:   detectAuthor(p.File),
				Date:     modTime.Format(time.RFC3339),
				Message:  commitMessage(p.File, base),
			})
		}
		manifest.Chains = append(manifest.Chains, chain)
	}

	out, err := yaml.Marshal(&manifest)
	if err != nil {
		log.Fatalf("marshal: %v", err)
	}
	os.Stdout.Write(out)
}

// sortByModTime sorts pages by their source file's modification time.
func sortByModTime(pages []PageEntry, sourceDir string) {
	sort.SliceStable(pages, func(i, j int) bool {
		ti := fileModTime(filepath.Join(sourceDir, pages[i].File))
		tj := fileModTime(filepath.Join(sourceDir, pages[j].File))
		return ti.Before(tj)
	})
}

// fileModTime returns the modification time of a file, or zero time on error.
func fileModTime(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// --- Ingest mode ---

func runIngest(manifestFile, dataDir, branch string, dryRun bool) {
	data, err := os.ReadFile(manifestFile)
	if err != nil {
		log.Fatalf("read manifest: %v", err)
	}
	var manifest ChainManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		log.Fatalf("parse manifest: %v", err)
	}

	proj := manifest.Project.ID
	if proj == "" {
		proj = "default"
	}
	sourceDir := manifest.Project.SourceDir

	if dryRun {
		fmt.Printf("DRY RUN — project %q, source: %s\n\n", proj, sourceDir)
		for _, c := range manifest.Chains {
			fmt.Printf("CHAIN: %s (%d versions)\n", c.BaseID, len(c.Versions))
			for _, v := range c.Versions {
				fmt.Printf("  → %s (%s, %s)\n", v.File, v.Author, v.Date)
			}
		}
		for _, s := range manifest.Standalone {
			fmt.Printf("STANDALONE: %s → %s\n", s.File, s.ID)
		}
		fmt.Println("\nNo changes made.")
		return
	}

	// Open or create the wiki git repo.
	repo, err := openOrInitRepo(dataDir, proj)
	if err != nil {
		log.Fatalf("open repo: %v", err)
	}

	// Create project page if needed.
	projectPagePath := fmt.Sprintf("pages/PROJECT-%s.md", proj)
	if !repo.HasPage(branch, projectPagePath) {
		if err := createProjectPage(repo, branch, manifest.Project); err != nil {
			log.Printf("WARN: could not create project page: %v", err)
		} else {
			fmt.Printf("[project] created PROJECT-%s\n", proj)
		}
	}

	total := len(manifest.Chains) + len(manifest.Standalone)
	idx := 0

	// Ingest chains: each chain becomes ONE page with sequential commits.
	for _, c := range manifest.Chains {
		idx++
		pageID := c.BaseID
		pagePath := fmt.Sprintf("pages/%s.md", pageID)

		for vi, v := range c.Versions {
			srcPath := filepath.Join(sourceDir, v.File)
			body, err := os.ReadFile(srcPath)
			if err != nil {
				log.Printf("SKIP %s: %v", v.File, err)
				continue
			}

			// Strip any existing frontmatter.
			_, rawBody, parseErr := schema.Parse(body)
			if parseErr != nil {
				rawBody = body
			}

			commitDate, err := time.Parse(time.RFC3339, v.Date)
			if err != nil {
				log.Printf("SKIP %s: bad date %q: %v", v.File, v.Date, err)
				continue
			}

			fm := &schema.Frontmatter{
				ID:         pageID,
				Title:      v.Title,
				Type:       v.Type,
				Status:     "draft",
				Tags:       v.Tags,
				Module:     v.Module,
				Category:   v.Category,
				ProjectRef: proj,
				DCCreator:  v.Author + "@prd2wiki",
				DCCreated:  schema.Date{Time: commitDate},
			}

			serialized, err := schema.Serialize(fm, rawBody)
			if err != nil {
				log.Printf("SKIP %s: serialize: %v", v.File, err)
				continue
			}

			msg := v.Message
			if msg == "" {
				msg = fmt.Sprintf("chain: %s version %d from %s", pageID, vi+1, filepath.Base(v.File))
			}
			author := v.Author + "@prd2wiki"

			err = repo.WritePageWithDate(branch, pagePath, serialized, msg, author, commitDate)
			if err != nil {
				log.Printf("FAIL %s: %v", v.File, err)
				continue
			}

			fmt.Printf("[%d/%d] chain %s v%d ← %s (%s, %s)\n",
				idx, total, pageID, vi+1, filepath.Base(v.File), v.Author, v.Date[:16])
		}
	}

	// Ingest standalone pages.
	for _, s := range manifest.Standalone {
		idx++
		srcPath := filepath.Join(sourceDir, s.File)
		body, err := os.ReadFile(srcPath)
		if err != nil {
			log.Printf("SKIP %s: %v", s.File, err)
			continue
		}

		_, rawBody, parseErr := schema.Parse(body)
		if parseErr != nil {
			rawBody = body
		}

		commitDate, err := time.Parse(time.RFC3339, s.Date)
		if err != nil {
			log.Printf("SKIP %s: bad date: %v", s.File, err)
			continue
		}

		fm := &schema.Frontmatter{
			ID:         s.ID,
			Title:      s.Title,
			Type:       s.Type,
			Status:     "draft",
			Tags:       s.Tags,
			Module:     s.Module,
			Category:   s.Category,
			ProjectRef: proj,
			DCCreator:  s.Author + "@prd2wiki",
			DCCreated:  schema.Date{Time: commitDate},
		}

		serialized, err := schema.Serialize(fm, rawBody)
		if err != nil {
			log.Printf("SKIP %s: serialize: %v", s.File, err)
			continue
		}

		pagePath := fmt.Sprintf("pages/%s.md", s.ID)
		msg := fmt.Sprintf("ingest: %s from %s", s.ID, s.File)
		author := s.Author + "@prd2wiki"

		err = repo.WritePageWithDate(branch, pagePath, serialized, msg, author, commitDate)
		if err != nil {
			log.Printf("FAIL %s: %v", s.File, err)
			continue
		}

		fmt.Printf("[%d/%d] standalone %s ← %s (%s)\n", idx, total, s.ID, s.File, s.Author)
	}

	fmt.Printf("\nDone. %d chains, %d standalone pages processed.\n", len(manifest.Chains), len(manifest.Standalone))
}

// openOrInitRepo opens an existing repo or initializes a new one.
func openOrInitRepo(dataDir, project string) (*wgit.Repo, error) {
	repo, err := wgit.OpenRepo(dataDir, project)
	if err != nil {
		repo, err = wgit.InitRepo(dataDir, project)
		if err != nil {
			return nil, fmt.Errorf("init repo: %w", err)
		}
	}
	return repo, nil
}

// createProjectPage creates a type=project page.
func createProjectPage(repo *wgit.Repo, branch string, info ManifestProject) error {
	fm := &schema.Frontmatter{
		ID:        "PROJECT-" + info.ID,
		Title:     info.ID,
		Type:      "project",
		Status:    "active",
		Tags:      []string{info.ID, "project"},
		Module:    "config",
		DCCreator: "chain-ingest@prd2wiki",
		DCCreated: schema.Date{Time: time.Now()},
	}

	body := fmt.Sprintf("# %s\n\nProject ingested from `%s`.\n", info.ID, info.SourceDir)

	serialized, err := schema.Serialize(fm, []byte(body))
	if err != nil {
		return err
	}

	pagePath := fmt.Sprintf("pages/PROJECT-%s.md", info.ID)
	return repo.WritePage(branch, pagePath, serialized, "ingest: create project page for "+info.ID, "chain-ingest@prd2wiki")
}

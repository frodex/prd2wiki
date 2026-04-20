// Phase B migration: assign page UUIDs, flatten to pages/{uuid}.md, write .link files.
// Run with wiki stopped. Expects repos at data/{project}.wiki.git -> symlinks to data/repos/proj_*.git
// and migration-manifest.json in data/.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/schema"
)

type manifest struct {
	WikiRoot string             `json:"wiki_root"`
	DataDir  string             `json:"data_dir"`
	Projects map[string]project `json:"projects"`
}

type project struct {
	UUID    string `json:"uuid"`
	Tree    string `json:"tree"`
	Display string `json:"display"`
}

var uuidV4 = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func main() {
	mPath := flag.String("manifest", "/srv/prd2wiki/data/migration-manifest.json", "manifest path")
	flag.Parse()
	raw, err := os.ReadFile(*mPath)
	if err != nil {
		log.Fatal(err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		log.Fatal(err)
	}
	dataDir := m.DataDir
	if dataDir == "" {
		dataDir = filepath.Join(m.WikiRoot, "data")
	}
	for name, p := range m.Projects {
		if err := migrateProject(dataDir, m.WikiRoot, name, p); err != nil {
			log.Fatalf("project %q: %v", name, err)
		}
	}
	log.Println("phase-b page migration: ok")
}

func isPageMarkdown(path string) bool {
	return strings.HasPrefix(path, "pages/") && strings.HasSuffix(path, ".md") && !strings.Contains(path, "/_attachments/")
}

func idFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".md")
}

func migrateProject(dataDir, wikiRoot, projName string, p project) error {
	repo, err := wgit.OpenRepo(dataDir, projName)
	if err != nil {
		return err
	}
	branches, err := repo.ListBranches()
	if err != nil {
		return err
	}
	type loc struct {
		branch, path string
	}
	var mdFiles []loc
	for _, b := range branches {
		paths, err := repo.ListPages(b)
		if err != nil {
			return fmt.Errorf("list pages %s: %w", b, err)
		}
		for _, path := range paths {
			if isPageMarkdown(path) {
				mdFiles = append(mdFiles, loc{b, path})
			}
		}
	}

	idToUUID := map[string]string{}
	titles := map[string]string{}

	for _, x := range mdFiles {
		fm, _, err := repo.ReadPageWithMeta(x.branch, x.path)
		if err != nil {
			return fmt.Errorf("read %s %s: %w", x.branch, x.path, err)
		}
		oldID := ""
		if fm != nil {
			oldID = strings.TrimSpace(fm.ID)
		}
		if oldID == "" {
			oldID = idFromPath(x.path)
		}
		title := oldID
		if fm != nil && strings.TrimSpace(fm.Title) != "" {
			title = strings.TrimSpace(fm.Title)
		}
		if titles[oldID] == "" {
			titles[oldID] = title
		}
		if _, ok := idToUUID[oldID]; ok {
			continue
		}
		if uuidV4.MatchString(oldID) && filepath.Base(x.path) == oldID+".md" {
			idToUUID[oldID] = oldID
		} else {
			idToUUID[oldID] = uuid.New().String()
		}
	}

	pathToNewUUID := map[string]string{}

	for _, x := range mdFiles {
		fm, body, err := repo.ReadPageWithMeta(x.branch, x.path)
		if err != nil {
			return err
		}
		if fm == nil {
			base := idFromPath(x.path)
			fm = &schema.Frontmatter{ID: base, Title: base, Type: "concept", Status: "draft"}
		}
		oldID := strings.TrimSpace(fm.ID)
		if oldID == "" {
			oldID = idFromPath(x.path)
		}
		newID := idToUUID[oldID]
		if newID == "" {
			continue
		}
		newPath := "pages/" + newID + ".md"
		fm.ID = newID
		raw, err := schema.Serialize(fm, body)
		if err != nil {
			return err
		}
		if x.path != newPath {
			if _, err = repo.WritePage(x.branch, newPath, raw, "phase-b: migrate to pages/{uuid}.md", "phase-b@prd2wiki"); err != nil {
				return fmt.Errorf("write %s: %w", newPath, err)
			}
			if err := repo.DeletePage(x.branch, x.path, "phase-b: remove old page path", "phase-b@prd2wiki"); err != nil {
				return fmt.Errorf("delete %s: %w", x.path, err)
			}
		} else {
			if _, err = repo.WritePage(x.branch, newPath, raw, "phase-b: normalize frontmatter id", "phase-b@prd2wiki"); err != nil {
				return fmt.Errorf("rewrite %s: %w", newPath, err)
			}
		}
		pathToNewUUID[x.path] = newID
	}

	// Attachment files (binary paths under pages/.../_attachments/)
	var attachFiles []loc
	for _, b := range branches {
		paths, err := repo.ListPages(b)
		if err != nil {
			return err
		}
		for _, path := range paths {
			if strings.Contains(path, "/_attachments/") && !strings.HasSuffix(path, ".md") {
				attachFiles = append(attachFiles, loc{b, path})
			}
		}
	}
	for _, a := range attachFiles {
		folder := attachmentFolderBeforeAttachments(a.path)
		newU := idToUUID[folder]
		if newU == "" {
			newU = pathToNewUUID["pages/"+folder+".md"]
		}
		if newU == "" {
			log.Printf("skip attachment (no parent page mapping): %s %s", a.branch, a.path)
			continue
		}
		parentDir := "pages/" + folder
		suffix := strings.TrimPrefix(a.path, parentDir+"/")
		newPath := "pages/" + newU + "/" + suffix
		data, err := repo.ReadPage(a.branch, a.path)
		if err != nil {
			return fmt.Errorf("read attachment %s: %w", a.path, err)
		}
		if _, err := repo.WritePage(a.branch, newPath, data, "phase-b: move attachment after uuid migrate", "phase-b@prd2wiki"); err != nil {
			return fmt.Errorf("write attachment %s: %w", newPath, err)
		}
		if err := repo.DeletePage(a.branch, a.path, "phase-b: remove old attachment path", "phase-b@prd2wiki"); err != nil {
			return fmt.Errorf("delete attachment %s: %w", a.path, err)
		}
	}

	treeDir := filepath.Join(wikiRoot, p.Tree)
	if err := os.MkdirAll(treeDir, 0o755); err != nil {
		return err
	}
	used := map[string]bool{}
	for oldID, pageUUID := range idToUUID {
		title := titles[oldID]
		if title == "" {
			title = oldID
		}
		slug := uniqueSlug(slugify(title, oldID), used)
		linkPath := filepath.Join(treeDir, slug+".link")
		content := pageUUID + "\n\n" + title + "\n"
		if err := os.WriteFile(linkPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write .link %s: %w", linkPath, err)
		}
	}
	return nil
}

// attachmentFolderBeforeAttachments returns the path segment between "pages/" and "/_attachments/".
// e.g. pages/6672420/_attachments/x.png -> "6672420" (matches frontmatter id even when the .md lives at pages/66/72420.md).
func attachmentFolderBeforeAttachments(attachmentPath string) string {
	const pref = "pages/"
	const sub = "/_attachments/"
	if !strings.HasPrefix(attachmentPath, pref) {
		return ""
	}
	rest := strings.TrimPrefix(attachmentPath, pref)
	i := strings.Index(rest, sub)
	if i < 0 {
		return ""
	}
	return rest[:i]
}

func slugify(title, fallback string) string {
	s := strings.ToLower(title)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(fallback), "-")
		s = strings.Trim(s, "-")
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

func uniqueSlug(base string, used map[string]bool) string {
	s := base
	n := 2
	for used[s] {
		s = fmt.Sprintf("%s-%d", base, n)
		n++
	}
	used[s] = true
	return s
}

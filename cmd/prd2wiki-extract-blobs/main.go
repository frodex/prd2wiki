// Extract attachment binaries to data/blobs/{aa}/{sha256} and rewrite markdown to /blobs/{sha256}.
// Run with wiki stopped. Expects migration-manifest.json (same projects as phase-b).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	wgit "github.com/frodex/prd2wiki/internal/git"
)

type manifest struct {
	DataDir  string             `json:"data_dir"`
	WikiRoot string             `json:"wiki_root"`
	Projects map[string]struct{} `json:"projects"`
}

var (
	apiAttach = regexp.MustCompile(`/api/projects/([a-zA-Z0-9_-]+)/pages/[^/\s)]+/attachments/([^)\s]+)`)
	// New layout (page UUID) or legacy short hex ids under pages/{id}/_attachments/
	pagesAttach = regexp.MustCompile(`pages/[a-f0-9-]+/_attachments/([^)\s\]]+)`)
)

func main() {
	mPath := flag.String("manifest", "/srv/prd2wiki/data/migration-manifest.json", "")
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
	blobsRoot := filepath.Join(dataDir, "blobs")

	for projName := range m.Projects {
		if err := processProject(dataDir, blobsRoot, projName); err != nil {
			log.Fatalf("project %q: %v", projName, err)
		}
	}
	log.Println("extract-blobs: ok")
}

func processProject(dataDir, blobsRoot, projName string) error {
	repo, err := wgit.OpenRepo(dataDir, projName)
	if err != nil {
		return err
	}
	branches, err := repo.ListBranches()
	if err != nil {
		return err
	}

	// basename -> sha256 hex (unique per project in our corpus)
	byName := map[string]string{}

	for _, branch := range branches {
		paths, err := repo.ListPages(branch)
		if err != nil {
			return err
		}
		for _, p := range paths {
			if !strings.Contains(p, "/_attachments/") || strings.HasSuffix(p, ".md") {
				continue
			}
			data, err := repo.ReadPage(branch, p)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			hexHash := hex.EncodeToString(sum[:])
			base := filepath.Base(p)
			if prev, ok := byName[base]; ok && prev != hexHash {
				log.Printf("warning: basename collision %s / %s vs %s in %s", base, prev, hexHash, projName)
			}
			byName[base] = hexHash

			sub := filepath.Join(blobsRoot, hexHash[:2], hexHash)
			if err := os.MkdirAll(filepath.Dir(sub), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(sub, data, 0o644); err != nil {
				return err
			}
		}
	}

	for _, branch := range branches {
		paths, err := repo.ListPages(branch)
		if err != nil {
			return err
		}
		for _, pagePath := range paths {
			if !strings.HasPrefix(pagePath, "pages/") || !strings.HasSuffix(pagePath, ".md") {
				continue
			}
			fm, body, err := repo.ReadPageWithMeta(branch, pagePath)
			if err != nil {
				return err
			}
			if fm == nil {
				continue
			}
			s := string(body)
			orig := s
			s = apiAttach.ReplaceAllStringFunc(s, func(match string) string {
				parts := apiAttach.FindStringSubmatch(match)
				if len(parts) != 3 {
					return match
				}
				if parts[1] != projName {
					return match
				}
				h := byName[parts[2]]
				if h == "" {
					return match
				}
				return "/blobs/" + h
			})
			s = pagesAttach.ReplaceAllStringFunc(s, func(match string) string {
				parts := pagesAttach.FindStringSubmatch(match)
				if len(parts) != 2 {
					return match
				}
				h := byName[parts[1]]
				if h == "" {
					return match
				}
				return "/blobs/" + h
			})
			if s == orig {
				continue
			}
			newBody := []byte(s)
			if _, err := repo.WritePageWithMeta(branch, pagePath, fm, newBody, "phase-b: reference /blobs/ for attachments", "phase-b@prd2wiki"); err != nil {
				return err
			}
		}
	}
	return nil
}

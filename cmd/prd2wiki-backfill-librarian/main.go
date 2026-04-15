// prd2wiki-backfill-librarian sends all wiki pages to pippi-librarian via memory_store.
// This populates the librarian's LanceDB after a schema wipe or fresh deployment.
//
// Usage:
//
//	prd2wiki-backfill-librarian -data ./data -tree ./tree -socket /var/run/pippi-librarian.sock
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/libclient"
	"github.com/frodex/prd2wiki/internal/schema"
	"github.com/frodex/prd2wiki/internal/tree"
)

func shard(uuid string) string {
	uuid = strings.TrimSpace(uuid)
	if len(uuid) >= 8 {
		return uuid[:8]
	}
	return uuid
}

func main() {
	dataDir := flag.String("data", "./data", "data directory (git repos)")
	treeDir := flag.String("tree", "./tree", "tree directory (.uuid/.link files)")
	socket := flag.String("socket", "/var/run/pippi-librarian.sock", "pippi-librarian unix socket")
	dryRun := flag.Bool("dry-run", false, "print what would be sent without calling librarian")
	flag.Parse()

	dataAbs, err := filepath.Abs(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "data dir: %v\n", err)
		os.Exit(1)
	}
	treeAbs, err := filepath.Abs(*treeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tree dir: %v\n", err)
		os.Exit(1)
	}

	// Scan tree to discover projects and pages.
	idx, err := tree.Scan(treeAbs, dataAbs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tree scan: %v\n", err)
		os.Exit(1)
	}

	// Connect to librarian via Unix socket with ticket auth.
	var cli *libclient.Client
	if !*dryRun {
		cli, err = libclient.New(*socket, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "librarian connect: %v\n", err)
			os.Exit(1)
		}
		cli.EnableTicketAuth([]string{"memory_store", "memory_search"})
	}

	// Build project UUID → repo key → repo mapping.
	repos := make(map[string]*wgit.Repo)
	for _, p := range idx.Projects {
		if p == nil || p.RepoKey == "" {
			continue
		}
		if _, ok := repos[p.RepoKey]; ok {
			continue
		}
		repo, err := wgit.OpenRepo(dataAbs, p.RepoKey)
		if err != nil {
			slog.Warn("skip project — cannot open repo", "project", p.RepoKey, "error", err)
			continue
		}
		repos[p.RepoKey] = repo
	}

	total := 0
	sent := 0
	skipped := 0
	failed := 0

	for _, ent := range idx.AllPageEntries() {
		total++
		if ent.Project == nil || ent.Page == nil {
			skipped++
			continue
		}

		pageUUID := ent.Page.UUID
		projectUUID := ent.Project.UUID
		repoKey := ent.Project.RepoKey
		if pageUUID == "" || projectUUID == "" || repoKey == "" {
			slog.Warn("skip page — missing UUID or repo key", "page", ent.Page.Slug)
			skipped++
			continue
		}

		repo, ok := repos[repoKey]
		if !ok {
			slog.Warn("skip page — no repo", "page", pageUUID, "repo_key", repoKey)
			skipped++
			continue
		}

		// Find the page in git — try branches until we find it.
		branches, err := repo.ListBranches()
		if err != nil {
			slog.Warn("skip page — cannot list branches", "page", pageUUID, "error", err)
			skipped++
			continue
		}

		var fm *schema.Frontmatter
		var body []byte
		var foundBranch string
		pagePath := fmt.Sprintf("pages/%s.md", strings.ToLower(pageUUID))
		for _, br := range branches {
			if !repo.HasPage(br, pagePath) {
				continue
			}
			fm, body, err = repo.ReadPageWithMeta(br, pagePath)
			if err == nil && fm != nil {
				foundBranch = br
				break
			}
		}
		if fm == nil {
			slog.Warn("skip page — not found in any branch", "page", pageUUID, "path", pagePath)
			skipped++
			continue
		}

		ns := "wiki:" + projectUUID
		meta := map[string]any{
			"page_title":  fm.Title,
			"page_type":   fm.Type,
			"page_status": fm.Status,
			"page_tags":   strings.Join(fm.Tags, ","),
			"author":      "backfill",
			"source_repo": "proj_" + shard(projectUUID) + ".git",
		}

		if *dryRun {
			fmt.Printf("[dry-run] %s ns=%s title=%q type=%s branch=%s body=%d bytes\n",
				pageUUID, ns, fm.Title, fm.Type, foundBranch, len(body))
			sent++
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		headID, err := cli.MemoryStore(ctx, ns, pageUUID, string(body), meta)
		cancel()

		if err != nil {
			slog.Error("memory_store failed", "page", pageUUID, "title", fm.Title, "error", err)
			failed++
			continue
		}

		sent++
		fmt.Printf("[%d/%d] %s → %s  %q\n", sent, total, pageUUID[:8], headID, fm.Title)
	}

	fmt.Printf("\nBackfill complete: %d sent, %d skipped, %d failed (of %d total)\n", sent, skipped, failed, total)
	if failed > 0 {
		os.Exit(1)
	}
}

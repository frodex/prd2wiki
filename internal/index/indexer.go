package index

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/schema"
)

// Indexer populates and rebuilds the SQLite index from git state.
type Indexer struct {
	db *sql.DB
}

// NewIndexer creates an Indexer backed by the given database.
func NewIndexer(db *sql.DB) *Indexer {
	return &Indexer{db: db}
}

// IndexPage upserts a single page into the computed index.
// It inserts/updates the pages table and replaces provenance_edges for the page.
func (ix *Indexer) IndexPage(project, branch, path string, fm *schema.Frontmatter, body []byte) error {
	// Format dates, using empty string for zero dates.
	dcCreated := ""
	if !fm.DCCreated.IsZero() {
		dcCreated = fm.DCCreated.Format("2006-01-02")
	}
	dcModified := ""
	if !fm.DCModified.IsZero() {
		dcModified = fm.DCModified.Format("2006-01-02")
	}

	tags := strings.Join(fm.Tags, ",")

	_, err := ix.db.Exec(`
		INSERT INTO pages (
			id, title, type, status, path, project, branch,
			trust_level, conformance,
			dc_creator, dc_created, dc_modified,
			supersedes, superseded_by, contested_by,
			tags, module, category, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			title        = excluded.title,
			type         = excluded.type,
			status       = excluded.status,
			path         = excluded.path,
			project      = excluded.project,
			branch       = excluded.branch,
			trust_level  = excluded.trust_level,
			conformance  = excluded.conformance,
			dc_creator   = excluded.dc_creator,
			dc_created   = excluded.dc_created,
			dc_modified  = excluded.dc_modified,
			supersedes   = excluded.supersedes,
			superseded_by = excluded.superseded_by,
			contested_by = excluded.contested_by,
			tags         = excluded.tags,
			module       = excluded.module,
			category     = excluded.category,
			updated_at   = CURRENT_TIMESTAMP
	`,
		fm.ID, fm.Title, fm.Type, fm.Status, path, project, branch,
		fm.TrustLevel, fm.Conformance,
		fm.DCCreator, dcCreated, dcModified,
		fm.Supersedes, fm.SupersededBy, fm.ContestedBy,
		tags, fm.Module, fm.Category,
	)
	if err != nil {
		return fmt.Errorf("upsert page %q: %w", fm.ID, err)
	}

	// Update FTS index — delete old entry, insert new one.
	// Body text strips link URLs so BM25 is not dominated by repeated /pages/<id> paths.
	_, _ = ix.db.Exec("DELETE FROM pages_fts WHERE id = ?", fm.ID)
	ftsBody := StripMarkdownForFTS(string(body))
	_, _ = ix.db.Exec(`INSERT INTO pages_fts (id, title, body, tags) VALUES (?, ?, ?, ?)`,
		fm.ID, fm.Title, ftsBody, tags)

	// Delete existing provenance edges for this page, then re-insert.
	if _, err := ix.db.Exec("DELETE FROM provenance_edges WHERE source_page = ?", fm.ID); err != nil {
		return fmt.Errorf("delete provenance edges for %q: %w", fm.ID, err)
	}

	for _, src := range fm.Provenance.Sources {
		_, err := ix.db.Exec(`
			INSERT INTO provenance_edges (source_page, target_ref, target_version, target_checksum, status)
			VALUES (?, ?, ?, ?, ?)
		`, fm.ID, src.Ref, src.Version, src.Checksum, src.Status)
		if err != nil {
			return fmt.Errorf("insert provenance edge (%q -> %q): %w", fm.ID, src.Ref, err)
		}
	}

	return nil
}

// RemovePage removes a page and its provenance edges from the index.
func (ix *Indexer) RemovePage(id string) error {
	_, _ = ix.db.Exec("DELETE FROM pages_fts WHERE id = ?", id)
	if _, err := ix.db.Exec("DELETE FROM provenance_edges WHERE source_page = ?", id); err != nil {
		return fmt.Errorf("delete provenance edges for %q: %w", id, err)
	}
	if _, err := ix.db.Exec("DELETE FROM pages WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete page %q: %w", id, err)
	}
	return nil
}

// RebuildFromRepo rebuilds the entire index for a project/branch by:
//  1. Deleting existing entries for the project
//  2. Listing all .md files on the branch
//  3. Reading and parsing each file
//  4. Calling IndexPage for each
func (ix *Indexer) RebuildFromRepo(project string, repo *wgit.Repo, branch string) error {
	// Clear existing entries for this project+branch only (not the whole project).
	_, err := ix.db.Exec(`
		DELETE FROM provenance_edges
		WHERE source_page IN (SELECT id FROM pages WHERE project = ? AND branch = ?)
	`, project, branch)
	if err != nil {
		return fmt.Errorf("clear provenance edges for project %q branch %q: %w", project, branch, err)
	}

	if _, err := ix.db.Exec("DELETE FROM pages WHERE project = ? AND branch = ?", project, branch); err != nil {
		return fmt.Errorf("clear pages for project %q branch %q: %w", project, branch, err)
	}

	// List all pages from git.
	paths, err := repo.ListPages(branch)
	if err != nil {
		return fmt.Errorf("list pages on branch %q: %w", branch, err)
	}

	for _, path := range paths {
		// Skip non-.md files.
		if !strings.HasSuffix(path, ".md") {
			continue
		}

		fm, body, err := repo.ReadPageWithMeta(branch, path)
		if err != nil {
			log.Printf("warning: RebuildFromRepo: failed to read %q: %v (skipping)", path, err)
			continue
		}
		if fm == nil {
			log.Printf("warning: RebuildFromRepo: no frontmatter in %q (skipping)", path)
			continue
		}

		if err := ix.IndexPage(project, branch, path, fm, body); err != nil {
			log.Printf("warning: RebuildFromRepo: failed to index %q: %v (skipping)", path, err)
			continue
		}
	}

	if err := ix.RecomputeLinkStats(project, branch, repo); err != nil {
		return fmt.Errorf("recompute link stats: %w", err)
	}

	return nil
}

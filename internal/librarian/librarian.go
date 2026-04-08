package librarian

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/schema"
	"github.com/frodex/prd2wiki/internal/vectordb"
	"github.com/frodex/prd2wiki/internal/vocabulary"
)

// PagePathOptions controls how pagePath resolves the storage path.
// It exists so callers can test hash-prefix vs legacy behavior.
type PagePathOptions struct {
	// ForceHashPrefix forces hash-prefix directory layout regardless of ID format.
	ForceHashPrefix bool
}

const (
	IntentVerbatim  = "verbatim"
	IntentConform   = "conform"
	IntentIntegrate = "integrate"
)

// SubmitRequest carries all inputs for a page submission.
type SubmitRequest struct {
	Project     string
	Branch      string
	Frontmatter *schema.Frontmatter
	Body        []byte
	Intent      string
	Author      string
}

// SubmitResult describes the outcome of a submission.
type SubmitResult struct {
	Saved    bool           `json:"saved"`
	Path     string         `json:"path"`
	Issues   []schema.Issue `json:"issues,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

// Librarian orchestrates page submission: validation, normalization, persistence, and indexing.
type Librarian struct {
	repo    *wgit.Repo
	indexer *index.Indexer
	vstore  *vectordb.Store
	vocab   *vocabulary.Store
}

// New creates a new Librarian.
func New(repo *wgit.Repo, indexer *index.Indexer, vstore *vectordb.Store, vocab *vocabulary.Store) *Librarian {
	return &Librarian{
		repo:    repo,
		indexer: indexer,
		vstore:  vstore,
		vocab:   vocab,
	}
}

// generateID creates a content-addressed hash ID from the title and current time,
// or a random fallback if the title is empty.
func generateID(title string) string {
	if title != "" {
		return schema.GeneratePageID(title, time.Now())
	}
	// Random fallback
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("page-%s-%x", time.Now().Format("20060102"), b)
}

// Submit processes a page submission according to the requested intent.
func (l *Librarian) Submit(ctx context.Context, req SubmitRequest) (*SubmitResult, error) {
	// Auto-generate ID if empty
	if req.Frontmatter.ID == "" {
		req.Frontmatter.ID = generateID(req.Frontmatter.Title)
	}

	// Extract base64 images on ALL intents — binary data doesn't belong in markdown.
	cleaned, extractedImages := ExtractBase64Images(string(req.Body), req.Frontmatter.ID, req.Project)
	if len(extractedImages) > 0 {
		req.Body = []byte(cleaned)
		// Store each extracted image as an attachment in git
		for _, img := range extractedImages {
			if err := l.repo.WritePage(req.Branch, img.Path, img.Data,
				"extract inline image: "+img.Filename, req.Author); err != nil {
				// Log but don't fail the whole submission
				continue
			}
		}
	}

	switch req.Intent {
	case IntentVerbatim:
		return l.submitVerbatim(ctx, req)
	case IntentConform:
		return l.submitConform(ctx, req)
	case IntentIntegrate:
		return l.submitIntegrate(ctx, req)
	default:
		return nil, fmt.Errorf("unknown intent %q", req.Intent)
	}
}

// SearchResult holds a search result with page ID and relevance.
type SearchResult struct {
	PageID     string  `json:"page_id"`
	Section    string  `json:"section,omitempty"`
	Similarity float64 `json:"similarity"`
}

// Search queries the vector store for pages matching the query text.
// The query is normalized through the vocabulary before searching.
func (l *Librarian) Search(ctx context.Context, project, query string, limit int) ([]SearchResult, error) {
	// Normalize query terms through vocabulary
	words := strings.Fields(query)
	normalized := make([]string, len(words))
	for i, w := range words {
		normalized[i] = l.vocab.Normalize(w)
	}
	normalizedQuery := strings.Join(normalized, " ")

	results, err := l.vstore.Search(ctx, project, normalizedQuery, limit)
	if err != nil {
		return nil, err
	}

	var out []SearchResult
	for _, r := range results {
		out = append(out, SearchResult{
			PageID:     r.PageID,
			Section:    r.Section,
			Similarity: r.Similarity,
		})
	}
	return out, nil
}

// FindSimilar finds pages similar to the given page via the vector store.
func (l *Librarian) FindSimilar(ctx context.Context, project, pageID string, limit int) ([]SearchResult, error) {
	results, err := l.vstore.FindSimilar(ctx, project, pageID, limit)
	if err != nil {
		return nil, err
	}

	var out []SearchResult
	for _, r := range results {
		out = append(out, SearchResult{
			PageID:     r.PageID,
			Section:    r.Section,
			Similarity: r.Similarity,
		})
	}
	return out, nil
}

// RebuildVectorIndex re-embeds all pages from a branch into the vector store.
func (l *Librarian) RebuildVectorIndex(ctx context.Context, project, branch string) (int, error) {
	pages, err := l.repo.ListPages(branch)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, path := range pages {
		if !strings.HasSuffix(path, ".md") {
			continue
		}
		fm, body, err := l.repo.ReadPageWithMeta(branch, path)
		if err != nil || fm == nil {
			continue
		}
		if err := l.indexInVectorStore(ctx, project, fm, body); err != nil {
			continue
		}
		count++
	}
	return count, nil
}

// pagePath returns the canonical git path for a page.
//
// For hash IDs (7 hex chars, no module/category): uses git-style hash-prefix
// directories: pages/{first-2-chars}/{rest}.md
//
// For human-readable IDs or pages with Module/Category: uses the original layout:
// pages/{module}/{category}/{id}.md or pages/{id}.md
//
// All path segments are sanitized to prevent traversal and injection attacks.
func pagePath(fm *schema.Frontmatter) string {
	id := schema.SanitizePathSegment(fm.ID)

	// If module or category is set, use the module/category layout (unchanged).
	if fm.Module != "" || fm.Category != "" {
		parts := []string{"pages"}
		if fm.Module != "" {
			parts = append(parts, schema.SanitizePathSegment(fm.Module))
		}
		if fm.Category != "" {
			parts = append(parts, schema.SanitizePathSegment(fm.Category))
		}
		parts = append(parts, id+".md")
		return strings.Join(parts, "/")
	}

	// Hash IDs get git-style hash-prefix directories.
	if schema.IsHashID(id) && len(id) >= 2 {
		return fmt.Sprintf("pages/%s/%s.md", id[:2], id[2:])
	}

	// Legacy human-readable IDs stay flat.
	return fmt.Sprintf("pages/%s.md", id)
}

// ResolvePagePath tries to find the actual storage path for a page ID.
// It checks the hash-prefix directory first, then falls back to the flat layout.
// The repo is checked to see which path actually has content.
// If neither exists, returns the canonical path for the ID format.
func ResolvePagePath(repo interface{ HasPage(branch, path string) bool }, branch, id string) string {
	sanitized := schema.SanitizePathSegment(id)

	// Try hash-prefix path first (for hash IDs).
	if len(sanitized) >= 2 {
		hashPath := fmt.Sprintf("pages/%s/%s.md", sanitized[:2], sanitized[2:])
		if repo != nil && repo.HasPage(branch, hashPath) {
			return hashPath
		}
	}

	// Try flat path (legacy).
	flatPath := fmt.Sprintf("pages/%s.md", sanitized)
	if repo != nil && repo.HasPage(branch, flatPath) {
		return flatPath
	}

	// Neither found — return canonical path for the ID format.
	if schema.IsHashID(sanitized) && len(sanitized) >= 2 {
		return fmt.Sprintf("pages/%s/%s.md", sanitized[:2], sanitized[2:])
	}
	return flatPath
}

// commitMessage builds the commit message for a submission.
func commitMessage(intent, title string) string {
	return fmt.Sprintf("submit (%s): %s", intent, title)
}

// indexInVectorStore indexes the page body into the vector store.
func (l *Librarian) indexInVectorStore(ctx context.Context, project string, fm *schema.Frontmatter, body []byte) error {
	chunks := ChunkByHeadings(string(body))
	if len(chunks) == 0 {
		// Fall back to a single chunk with the whole body.
		chunks = []vectordb.TextChunk{
			{Section: fm.Title, Text: string(body)},
		}
	}
	tags := ""
	if len(fm.Tags) > 0 {
		for i, t := range fm.Tags {
			if i > 0 {
				tags += ","
			}
			tags += t
		}
	}
	return l.vstore.IndexPage(ctx, project, fm.ID, fm.Type, tags, chunks)
}

// submitVerbatim handles verbatim submissions.
func (l *Librarian) submitVerbatim(ctx context.Context, req SubmitRequest) (*SubmitResult, error) {
	// Validate — flag issues but do not block.
	issues := schema.Validate(req.Frontmatter)
	if schema.HasErrors(issues) {
		req.Frontmatter.Conformance = "pending"
	} else {
		req.Frontmatter.Conformance = "valid"
	}

	path := pagePath(req.Frontmatter)
	msg := commitMessage(IntentVerbatim, req.Frontmatter.Title)

	if err := l.repo.WritePageWithMeta(req.Branch, path, req.Frontmatter, req.Body, msg, req.Author); err != nil {
		return nil, fmt.Errorf("write page: %w", err)
	}

	if err := l.indexer.IndexPage(req.Project, req.Branch, path, req.Frontmatter, req.Body); err != nil {
		return nil, fmt.Errorf("index page: %w", err)
	}

	if err := l.indexInVectorStore(ctx, req.Project, req.Frontmatter, req.Body); err != nil {
		// Non-fatal: log but continue.
		_ = err
	}

	return &SubmitResult{
		Saved:  true,
		Path:   path,
		Issues: issues,
	}, nil
}

// submitConform handles conform submissions.
func (l *Librarian) submitConform(ctx context.Context, req SubmitRequest) (*SubmitResult, error) {
	// Validate — block on errors.
	issues := schema.Validate(req.Frontmatter)
	if schema.HasErrors(issues) {
		return &SubmitResult{
			Saved:  false,
			Issues: issues,
		}, nil
	}

	// Normalize tags.
	normalized := make([]string, len(req.Frontmatter.Tags))
	for i, tag := range req.Frontmatter.Tags {
		n := l.vocab.Normalize(tag)
		normalized[i] = n
		_ = l.vocab.Add(n, "tag")
	}
	req.Frontmatter.Tags = normalized

	// Normalize body.
	normalizedBody := NormalizeMarkdown(string(req.Body))
	req.Frontmatter.Conformance = "valid"

	path := pagePath(req.Frontmatter)
	msg := commitMessage(IntentConform, req.Frontmatter.Title)

	if err := l.repo.WritePageWithMeta(req.Branch, path, req.Frontmatter, []byte(normalizedBody), msg, req.Author); err != nil {
		return nil, fmt.Errorf("write page: %w", err)
	}

	if err := l.indexer.IndexPage(req.Project, req.Branch, path, req.Frontmatter, []byte(normalizedBody)); err != nil {
		return nil, fmt.Errorf("index page: %w", err)
	}

	if err := l.indexInVectorStore(ctx, req.Project, req.Frontmatter, []byte(normalizedBody)); err != nil {
		_ = err
	}

	return &SubmitResult{
		Saved:  true,
		Path:   path,
		Issues: issues,
	}, nil
}

// submitIntegrate handles integrate submissions (conform + dedup check).
func (l *Librarian) submitIntegrate(ctx context.Context, req SubmitRequest) (*SubmitResult, error) {
	// Validate — block on errors.
	issues := schema.Validate(req.Frontmatter)
	if schema.HasErrors(issues) {
		return &SubmitResult{
			Saved:  false,
			Issues: issues,
		}, nil
	}

	// Normalize tags.
	normalized := make([]string, len(req.Frontmatter.Tags))
	for i, tag := range req.Frontmatter.Tags {
		n := l.vocab.Normalize(tag)
		normalized[i] = n
		_ = l.vocab.Add(n, "tag")
	}
	req.Frontmatter.Tags = normalized

	// Normalize body.
	normalizedBody := NormalizeMarkdown(string(req.Body))
	req.Frontmatter.Conformance = "valid"

	path := pagePath(req.Frontmatter)
	msg := commitMessage(IntentIntegrate, req.Frontmatter.Title)

	if err := l.repo.WritePageWithMeta(req.Branch, path, req.Frontmatter, []byte(normalizedBody), msg, req.Author); err != nil {
		return nil, fmt.Errorf("write page: %w", err)
	}

	if err := l.indexer.IndexPage(req.Project, req.Branch, path, req.Frontmatter, []byte(normalizedBody)); err != nil {
		return nil, fmt.Errorf("index page: %w", err)
	}

	if err := l.indexInVectorStore(ctx, req.Project, req.Frontmatter, []byte(normalizedBody)); err != nil {
		_ = err
	}

	// Dedup check.
	detector := NewDedupDetector(l.vstore)
	dedupResult, err := detector.Check(ctx, req.Project, req.Frontmatter.ID, normalizedBody)
	var warnings []string
	if err == nil && dedupResult != nil {
		for _, c := range dedupResult.Candidates {
			warnings = append(warnings, fmt.Sprintf("potential duplicate: %s (similarity: %.2f)", c.PageID, c.Similarity))
		}
	}

	return &SubmitResult{
		Saved:    true,
		Path:     path,
		Issues:   issues,
		Warnings: warnings,
	}, nil
}

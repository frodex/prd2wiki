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

// generateID creates a page ID from the title, or a random one if no title.
func generateID(title string) string {
	if title != "" {
		// Slugify the title
		id := strings.ToLower(title)
		id = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}
			if r == ' ' || r == '-' || r == '_' {
				return '-'
			}
			return -1
		}, id)
		// Collapse multiple dashes
		for strings.Contains(id, "--") {
			id = strings.ReplaceAll(id, "--", "-")
		}
		id = strings.Trim(id, "-")
		if id != "" {
			return id
		}
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
func pagePath(id string) string {
	return "pages/" + id + ".md"
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

	path := pagePath(req.Frontmatter.ID)
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

	path := pagePath(req.Frontmatter.ID)
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

	path := pagePath(req.Frontmatter.ID)
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

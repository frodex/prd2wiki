package librarian

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"strings"
	"time"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/libclient"
	"github.com/frodex/prd2wiki/internal/tree"
	"github.com/google/uuid"

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
	// PageUUID and ProjectUUID are optional until tree migration; when empty,
	// syncToLibrarian (future libclient) is a no-op.
	PageUUID    string
	ProjectUUID string
	// UseFlatUUIDPath stores the page at pages/{uuid}.md (tree-backed pages). Ignores module/category layout.
	UseFlatUUIDPath bool
}

// SubmitResult describes the outcome of a submission.
type SubmitResult struct {
	Saved      bool           `json:"saved"`
	Path       string         `json:"path"`
	Issues     []schema.Issue `json:"issues,omitempty"`
	Warnings   []string       `json:"warnings,omitempty"`
	CommitHash string         `json:"commit_hash,omitempty"`
}

// submitFlags selects behavior for the unified submit path.
type submitFlags struct {
	blockOnErrors bool
	normalize     bool
	dedup         bool
	logVectorWarn bool
}

func submitFlagsForIntent(intent string) (submitFlags, error) {
	switch intent {
	case IntentVerbatim:
		return submitFlags{logVectorWarn: true}, nil
	case IntentConform:
		return submitFlags{blockOnErrors: true, normalize: true}, nil
	case IntentIntegrate:
		return submitFlags{blockOnErrors: true, normalize: true, dedup: true}, nil
	default:
		return submitFlags{}, fmt.Errorf("unknown intent %q", intent)
	}
}

// Librarian orchestrates page submission: validation, normalization, persistence, and indexing.
type Librarian struct {
	repo    *wgit.Repo
	indexer *index.Indexer
	vstore  *vectordb.Store
	vocab   *vocabulary.Store

	libClient  *libclient.Client
	treeHolder *tree.IndexHolder
}

// Option configures optional integrations (e.g. pippi-librarian sync).
type Option func(*Librarian)

// WithPippiLibrarian enables async push to pippi-librarian and .link line 2 updates when cli is non-nil.
func WithPippiLibrarian(cli *libclient.Client, holder *tree.IndexHolder) Option {
	return func(l *Librarian) {
		l.libClient = cli
		l.treeHolder = holder
	}
}

// New creates a new Librarian.
func New(repo *wgit.Repo, indexer *index.Indexer, vstore *vectordb.Store, vocab *vocabulary.Store, opts ...Option) *Librarian {
	l := &Librarian{
		repo:    repo,
		indexer: indexer,
		vstore:  vstore,
		vocab:   vocab,
	}
	for _, o := range opts {
		o(l)
	}
	return l
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
		if req.UseFlatUUIDPath {
			req.Frontmatter.ID = uuid.New().String()
		} else {
			req.Frontmatter.ID = generateID(req.Frontmatter.Title)
		}
	}

	// Extract base64 images on ALL intents — binary data doesn't belong in markdown.
	cleaned, extractedImages := ExtractBase64Images(string(req.Body), req.Frontmatter.ID, req.Project)
	if len(extractedImages) > 0 {
		req.Body = []byte(cleaned)
		// Store each extracted image as an attachment in git
		for _, img := range extractedImages {
			if _, err := l.repo.WritePage(req.Branch, img.Path, img.Data,
				"extract inline image: "+img.Filename, req.Author); err != nil {
				// Log but don't fail the whole submission
				continue
			}
		}
	}

	return l.submit(ctx, req)
}

func projectGitShard(projectUUID string) string {
	projectUUID = strings.TrimSpace(projectUUID)
	if len(projectUUID) >= 8 {
		return projectUUID[:8]
	}
	return projectUUID
}

// maybeSyncToLibrarian pushes the page to pippi-librarian (async) when configured.
func (l *Librarian) maybeSyncToLibrarian(ctx context.Context, req SubmitRequest, res *SubmitResult) {
	if l == nil || l.libClient == nil || l.treeHolder == nil || res == nil || !res.Saved {
		return
	}
	pageUUID := strings.TrimSpace(req.PageUUID)
	if pageUUID == "" && req.Frontmatter != nil {
		pageUUID = strings.TrimSpace(req.Frontmatter.ID)
	}
	projectUUID := strings.TrimSpace(req.ProjectUUID)
	if projectUUID == "" || pageUUID == "" {
		return
	}
	_ = ctx
	commitHash := res.CommitHash
	reqCopy := req
	go l.runSyncToLibrarian(reqCopy, pageUUID, projectUUID, commitHash)
}

func (l *Librarian) runSyncToLibrarian(req SubmitRequest, pageUUID, projectUUID, commitHash string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ext := map[string]any{
		"source_repo":   "proj_" + projectGitShard(projectUUID) + ".git",
		"source_branch": req.Branch,
		"source_commit": commitHash,
		"author":        req.Author,
	}
	if req.Frontmatter != nil {
		ext["page_title"] = req.Frontmatter.Title
		ext["page_type"] = req.Frontmatter.Type
		ext["page_status"] = req.Frontmatter.Status
		ext["page_tags"] = strings.Join(req.Frontmatter.Tags, ",")
	}

	ns := "wiki:" + projectUUID
	headID, err := l.libClient.MemoryStore(ctx, ns, pageUUID, string(req.Body), ext)
	if err != nil {
		slog.Warn("librarian sync failed", "page", pageUUID, "err", err)
		return
	}
	if err := l.treeHolder.UpdateLibrarianHeadInLink(pageUUID, headID); err != nil {
		slog.Warn("librarian .link line 2 update failed", "page", pageUUID, "err", err)
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
	failed := 0
	consecutiveFails := 0
	for _, path := range pages {
		if !strings.HasSuffix(path, ".md") {
			continue
		}
		// Stop if embedder appears dead (many consecutive failures)
		if consecutiveFails >= 5 {
			slog.Error("vector rebuild: stopping — embedder appears down", "project", project, "branch", branch, "consecutive_failures", consecutiveFails)
			break
		}
		fm, body, err := l.repo.ReadPageWithMeta(branch, path)
		if err != nil || fm == nil {
			slog.Warn("vector rebuild: skip unreadable page", "path", path, "error", err)
			failed++
			continue
		}
		if err := l.indexInVectorStore(ctx, project, fm, body); err != nil {
			slog.Warn("vector rebuild: embed failed", "page", fm.ID, "path", path, "error", err)
			failed++
			consecutiveFails++
			continue
		}
		consecutiveFails = 0 // reset on success
		count++
	}
	if failed > 0 {
		slog.Warn("vector rebuild: some pages failed", "project", project, "branch", branch, "ok", count, "failed", failed)
	}
	return count, nil
}

// RemoveFromIndexes drops a page from SQLite and vector indexes without modifying git.
func (l *Librarian) RemoveFromIndexes(pageID string) error {
	if err := l.indexer.RemovePage(pageID); err != nil {
		return err
	}
	l.vstore.RemovePage(pageID)
	return nil
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
	if schema.IsUUIDPageID(fm.ID) {
		return fmt.Sprintf("pages/%s.md", strings.ToLower(strings.TrimSpace(fm.ID)))
	}

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
func ResolvePagePath(repo interface {
	HasPage(branch, path string) bool
}, branch, id string) string {
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
// Title and tags are prepended to each chunk so vector similarity
// matches on page identity, not just body content.
func (l *Librarian) indexInVectorStore(ctx context.Context, project string, fm *schema.Frontmatter, body []byte) error {
	chunks := ChunkByHeadings(string(body))
	if len(chunks) == 0 {
		chunks = []vectordb.TextChunk{
			{Section: fm.Title, Text: string(body)},
		}
	}
	// Prepend title + tags to each chunk for better search relevance.
	// Without this, vector search only matches on body content and misses
	// pages where the title/tags are the best match for the query.
	prefix := fm.Title
	if len(fm.Tags) > 0 {
		prefix += " " + strings.Join(fm.Tags, " ")
	}
	if fm.Type != "" {
		prefix += " " + fm.Type
	}
	for i := range chunks {
		chunks[i].Text = prefix + "\n\n" + chunks[i].Text
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

// submit persists a page for verbatim, conform, or integrate intents.
func (l *Librarian) submit(ctx context.Context, req SubmitRequest) (*SubmitResult, error) {
	flags, err := submitFlagsForIntent(req.Intent)
	if err != nil {
		return nil, err
	}

	issues := schema.Validate(req.Frontmatter)

	if flags.blockOnErrors && schema.HasErrors(issues) {
		return &SubmitResult{
			Saved:  false,
			Issues: issues,
		}, nil
	}

	if !flags.blockOnErrors {
		if schema.HasErrors(issues) {
			req.Frontmatter.Conformance = "pending"
		} else {
			req.Frontmatter.Conformance = "valid"
		}
	} else {
		normalizedTags := make([]string, len(req.Frontmatter.Tags))
		for i, tag := range req.Frontmatter.Tags {
			n := l.vocab.Normalize(tag)
			normalizedTags[i] = n
			_ = l.vocab.Add(n, "tag")
		}
		req.Frontmatter.Tags = normalizedTags
		req.Frontmatter.Conformance = "valid"
	}

	bodyToWrite := req.Body
	if flags.normalize {
		bodyToWrite = []byte(NormalizeMarkdown(string(req.Body)))
	}

	var path string
	if req.UseFlatUUIDPath {
		if !schema.IsUUIDPageID(req.Frontmatter.ID) {
			return nil, fmt.Errorf("UseFlatUUIDPath requires a UUID page id")
		}
		path = fmt.Sprintf("pages/%s.md", strings.ToLower(strings.TrimSpace(req.Frontmatter.ID)))
	} else {
		path = pagePath(req.Frontmatter)
	}
	msg := commitMessage(req.Intent, req.Frontmatter.Title)

	commitHash, err := l.repo.WritePageWithMeta(req.Branch, path, req.Frontmatter, bodyToWrite, msg, req.Author)
	if err != nil {
		return nil, fmt.Errorf("write page: %w", err)
	}

	if err := l.indexer.IndexPage(req.Project, req.Branch, path, req.Frontmatter, bodyToWrite); err != nil {
		return nil, fmt.Errorf("index page: %w", err)
	}

	// BUG-012/BUG-014: Vector embedding + dedup async — don't block the response.
	// Git commit + SQLite index are done. Vector index updates in background.
	go func() {
		bgCtx := context.Background()
		if err := l.indexInVectorStore(bgCtx, req.Project, req.Frontmatter, bodyToWrite); err != nil {
			slog.Warn("vector index failed (async)", "page", req.Frontmatter.ID, "error", err)
		}
	}()

	var warnings []string

	res := &SubmitResult{
		Saved:      true,
		Path:       path,
		Issues:     issues,
		Warnings:   warnings,
		CommitHash: commitHash,
	}
	l.maybeSyncToLibrarian(ctx, req, res)
	return res, nil
}

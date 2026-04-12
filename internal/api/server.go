package api

import (
	"database/sql"
	"fmt"
	"net/http"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/schema"
	"github.com/frodex/prd2wiki/internal/web"
)

// Server holds application state and serves the REST API.
type Server struct {
	addr       string
	repos      map[string]*wgit.Repo
	db         *sql.DB
	indexer    *index.Indexer
	search     *index.Searcher
	librarians map[string]*librarian.Librarian
	edits      map[string]*web.EditCache
}

// NewServer creates a Server with the given address, repos, database, and librarians.
// All vector search and content operations go through the librarians.
func NewServer(addr string, repos map[string]*wgit.Repo, db *sql.DB, librarians map[string]*librarian.Librarian, edits map[string]*web.EditCache) *Server {
	return &Server{
		addr:       addr,
		repos:      repos,
		db:         db,
		indexer:    index.NewIndexer(db),
		search:     index.NewSearcher(db),
		librarians: librarians,
		edits:      edits,
	}
}

// Handler returns an http.Handler with all API routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/projects/{project}/pages", s.createPage)
	mux.HandleFunc("GET /api/projects/{project}/pages/{id...}", s.getPage)
	mux.HandleFunc("PUT /api/projects/{project}/pages/{id...}", s.updatePage)
	mux.HandleFunc("DELETE /api/projects/{project}/pages/{id...}", s.deletePage)
	mux.HandleFunc("GET /api/projects/{project}/pages", s.listPages)
	mux.HandleFunc("GET /api/projects/{project}/search", s.searchPages)
	mux.HandleFunc("GET /api/projects/{project}/pages/{id}/references", s.getReferences)
	mux.HandleFunc("GET /api/projects/{project}/pages/{id}/history", s.pageHistory)
	mux.HandleFunc("GET /api/projects/{project}/pages/{id}/history/{hash}", s.pageAtCommit)
	mux.HandleFunc("GET /api/projects/{project}/pages/{id}/diff", s.pageDiff)
	mux.HandleFunc("POST /api/projects/{project}/pages/{id}/deprecate", s.deprecatePage)
	mux.HandleFunc("POST /api/projects/{project}/pages/{id}/restore", s.restorePage)
	mux.HandleFunc("POST /api/projects/{project}/pages/{id}/approve", s.approvePage)

	mux.HandleFunc("POST /api/projects/{project}/pages/{id}/attachments", s.uploadAttachment)
	mux.HandleFunc("GET /api/projects/{project}/pages/{id}/attachments", s.listAttachments)
	mux.HandleFunc("GET /api/projects/{project}/pages/{id}/attachments/{filename}", s.getAttachment)

	return mux
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.addr, s.Handler())
}

// resolvePagePath looks up the stored path for a page ID from the SQLite index.
// Falls back to hash-prefix path for hash IDs, or flat path for legacy IDs.
func (s *Server) resolvePagePath(project, id string) string {
	results, err := s.search.ByID(project, id)
	if err == nil && len(results) > 0 && results[0].Path != "" {
		return results[0].Path
	}
	sanitized := schema.SanitizePathSegment(id)
	if schema.IsHashID(sanitized) && len(sanitized) >= 2 {
		return fmt.Sprintf("pages/%s/%s.md", sanitized[:2], sanitized[2:])
	}
	return fmt.Sprintf("pages/%s.md", sanitized)
}

// alternatePagePath returns the other path format for an ID (hash-prefix vs flat).
// Returns "" if there is no meaningful alternate.
func (s *Server) alternatePagePath(id, currentPath string) string {
	sanitized := schema.SanitizePathSegment(id)
	flat := fmt.Sprintf("pages/%s.md", sanitized)
	if len(sanitized) >= 2 {
		hashPrefix := fmt.Sprintf("pages/%s/%s.md", sanitized[:2], sanitized[2:])
		if currentPath == hashPrefix {
			return flat
		}
		return hashPrefix
	}
	return ""
}

// Package api serves JSON and binary resources under /api/ only. The application root mux
// may register other top-level routes (wiki tree URLs, static files); those must use reserved
// prefixes so they are not conflated with API handlers — see internal/tree.IsReservedRequestPath.
package api

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/frodex/prd2wiki/internal/auth"
	"github.com/frodex/prd2wiki/internal/blob"
	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/pagepath"
	"github.com/frodex/prd2wiki/internal/tree"
	"github.com/frodex/prd2wiki/internal/web"
)

// ServerConfig wires the API server (repos, index, tree holder for CRUD, optional blobs and keys).
type ServerConfig struct {
	Addr       string
	Repos      map[string]*wgit.Repo
	DB         *sql.DB
	Librarians map[string]*librarian.Librarian
	Edits      map[string]*web.EditCache
	Tree       *tree.IndexHolder
	Blob       *blob.Store
	Keys       *auth.ServiceKeyStore
	// MigrationAliases maps post-migration git paths to prior paths (data/migration-map.json).
	MigrationAliases map[string][]string
}

// Server holds application state and serves the REST API.
type Server struct {
	addr       string
	repos      map[string]*wgit.Repo
	db         *sql.DB
	indexer    *index.Indexer
	search     *index.Searcher
	librarians map[string]*librarian.Librarian
	edits      map[string]*web.EditCache
	treeHolder *tree.IndexHolder
	blobStore  *blob.Store
	keys       *auth.ServiceKeyStore
	migrationAliases map[string][]string
}

// NewServer creates a Server from config.
func NewServer(cfg ServerConfig) *Server {
	return &Server{
		addr:             cfg.Addr,
		repos:            cfg.Repos,
		db:               cfg.DB,
		indexer:          index.NewIndexer(cfg.DB),
		search:           index.NewSearcher(cfg.DB),
		librarians:       cfg.Librarians,
		edits:            cfg.Edits,
		treeHolder:       cfg.Tree,
		blobStore:        cfg.Blob,
		keys:             cfg.Keys,
		migrationAliases: cfg.MigrationAliases,
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

	mux.HandleFunc("GET /api/tree", s.handleTreeAPI)
	mux.HandleFunc("GET /api/tree/{path...}", s.handleTreeAPI)
	mux.HandleFunc("POST /api/tree/{path...}", s.handleTreeAPI)
	mux.HandleFunc("PUT /api/tree/{path...}", s.handleTreeAPI)
	mux.HandleFunc("DELETE /api/tree/{path...}", s.handleTreeAPI)

	mux.HandleFunc("POST /api/blobs", s.postBlob)

	return s.wrapProjectsAPILegacyRedirect(mux)
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.addr, s.Handler())
}

// projectRepo returns the git repo for project or writes 404 and false.
func (s *Server) projectRepo(w http.ResponseWriter, project string) (*wgit.Repo, bool) {
	repo, ok := s.repos[project]
	if !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
		return nil, false
	}
	return repo, true
}

// projectLibrarian returns the librarian for project or writes 404 and false.
func (s *Server) projectLibrarian(w http.ResponseWriter, project string) (*librarian.Librarian, bool) {
	lib, ok := s.librarians[project]
	if !ok {
		http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
		return nil, false
	}
	return lib, true
}

// resolvePagePath looks up the stored path for a page ID from the SQLite index.
// Falls back to hash-prefix path for hash IDs, or flat path for legacy IDs.
func (s *Server) resolvePagePath(project, id string) string {
	return pagepath.Resolve(s.search, project, id)
}

func (s *Server) aliasPathsFor(gitPath string) []string {
	if s == nil || len(s.migrationAliases) == 0 {
		return nil
	}
	return s.migrationAliases[gitPath]
}

// alternatePagePath returns the other path format for an ID (hash-prefix vs flat).
// Returns "" if there is no meaningful alternate.
func (s *Server) alternatePagePath(id, currentPath string) string {
	return pagepath.Alternate(id, currentPath)
}

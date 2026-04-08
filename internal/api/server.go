package api

import (
	"database/sql"
	"net/http"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
)

// Server holds application state and serves the REST API.
type Server struct {
	addr       string
	repos      map[string]*wgit.Repo
	db         *sql.DB
	indexer    *index.Indexer
	search     *index.Searcher
	librarians map[string]*librarian.Librarian
}

// NewServer creates a Server with the given address, repos, database, and librarians.
func NewServer(addr string, repos map[string]*wgit.Repo, db *sql.DB, librarians map[string]*librarian.Librarian) *Server {
	return &Server{
		addr:       addr,
		repos:      repos,
		db:         db,
		indexer:    index.NewIndexer(db),
		search:     index.NewSearcher(db),
		librarians: librarians,
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

	return mux
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.addr, s.Handler())
}

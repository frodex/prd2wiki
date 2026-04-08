package api

import (
	"database/sql"
	"net/http"

	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
)

// Server holds application state and serves the REST API.
type Server struct {
	addr    string
	repos   map[string]*wgit.Repo
	db      *sql.DB
	indexer *index.Indexer
	search  *index.Searcher
}

// NewServer creates a Server with the given address, repos, and database.
func NewServer(addr string, repos map[string]*wgit.Repo, db *sql.DB) *Server {
	return &Server{
		addr:    addr,
		repos:   repos,
		db:      db,
		indexer: index.NewIndexer(db),
		search:  index.NewSearcher(db),
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

	return mux
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.addr, s.Handler())
}

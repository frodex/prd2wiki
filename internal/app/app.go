package app

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/frodex/prd2wiki/internal/api"
	"github.com/frodex/prd2wiki/internal/auth"
	"github.com/frodex/prd2wiki/internal/blob"
	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/libclient"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/tree"
	"github.com/frodex/prd2wiki/internal/vocabulary"
	"github.com/frodex/prd2wiki/internal/web"
)

// Config holds all application configuration.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Data      DataConfig      `yaml:"data"`
	Tree      TreeConfig      `yaml:"tree"`
	Librarian LibrarianConfig `yaml:"librarian"`
	// Projects is deprecated: projects are discovered from the tree scan. Ignored if present.
	Projects []string `yaml:"projects"`
}

// TreeConfig holds the on-disk wiki tree directory (UUID projects and .link pages).
type TreeConfig struct {
	Dir string `yaml:"dir"`
}

// LibrarianConfig holds optional librarian integration settings (used in later phases).
type LibrarianConfig struct {
	Socket string `yaml:"socket"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Addr string `yaml:"addr"`
}

// DataConfig holds data directory settings.
type DataConfig struct {
	Dir string `yaml:"dir"`
}

// App holds all initialized services and the HTTP handler.
type App struct {
	Config     Config
	Repos      map[string]*wgit.Repo
	DB         *sql.DB
	Indexer    *index.Indexer
	Searcher   *index.Searcher
	Librarians map[string]*librarian.Librarian
	Keys       *auth.ServiceKeyStore
	Handler    http.Handler
}

// New creates and wires all services from the config.
func New(cfg Config) (*App, error) {
	// Apply defaults.
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}
	if cfg.Data.Dir == "" {
		cfg.Data.Dir = "./data"
	}
	if cfg.Tree.Dir == "" {
		cfg.Tree.Dir = "./tree"
	}

	dataAbs, err := filepath.Abs(cfg.Data.Dir)
	if err != nil {
		return nil, fmt.Errorf("data dir: %w", err)
	}
	treeAbs, err := filepath.Abs(cfg.Tree.Dir)
	if err != nil {
		return nil, fmt.Errorf("tree dir: %w", err)
	}

	// Create data directory if needed.
	if err := os.MkdirAll(dataAbs, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir %q: %w", dataAbs, err)
	}

	treeIdx, err := tree.Scan(treeAbs, dataAbs)
	if err != nil {
		return nil, fmt.Errorf("tree scan: %w", err)
	}
	if len(treeIdx.Projects) == 0 {
		return nil, fmt.Errorf("no projects under tree %q (need directories with .uuid)", treeAbs)
	}

	treeHolder := tree.NewIndexHolder(treeAbs, dataAbs, treeIdx)

	// Stable unique repo keys from the tree (e.g. default, battletech).
	seen := make(map[string]bool)
	var projectKeys []string
	for _, p := range treeIdx.Projects {
		if p.RepoKey == "" || seen[p.RepoKey] {
			continue
		}
		seen[p.RepoKey] = true
		projectKeys = append(projectKeys, p.RepoKey)
	}
	sort.Strings(projectKeys)
	for _, pk := range projectKeys {
		slog.Info("discovered project from tree", "repo_key", pk)
	}

	// Open or initialize git bare repos for each discovered project.
	repos := make(map[string]*wgit.Repo, len(projectKeys))
	for _, project := range projectKeys {
		repo, err := wgit.OpenRepo(dataAbs, project)
		if err != nil {
			// Repo doesn't exist yet -- initialize it.
			repo, err = wgit.InitRepo(dataAbs, project)
			if err != nil {
				return nil, fmt.Errorf("init repo %q: %w", project, err)
			}
			slog.Info("initialized new repo", "project", project)
		} else {
			slog.Info("opened repo", "project", project)
		}
		repos[project] = repo
	}

	// Open SQLite index database.
	dbPath := filepath.Join(dataAbs, "index.db")
	db, err := index.OpenDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", dbPath, err)
	}

	// Initialize service key store (uses the same DB, migrations are idempotent).
	keyStore, err := auth.NewServiceKeyStore(db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("init service key store: %w", err)
	}

	// Rebuild index from repos on startup (sequential — SQLite only allows one writer).
	indexer := index.NewIndexer(db)
	for _, project := range projectKeys {
		repo := repos[project]
		branches, err := repo.ListBranches()
		if err != nil {
			slog.Warn("list branches failed, skipping rebuild", "project", project, "error", err)
			continue
		}
		for _, branch := range branches {
			slog.Info("rebuilding index", "project", project, "branch", branch)
			if err := indexer.RebuildFromRepo(project, repo, branch); err != nil {
				slog.Warn("index rebuild error", "project", project, "branch", branch, "error", err)
			}
		}
	}

	var pippi *libclient.Client
	var pippiDialErr error
	if socket := strings.TrimSpace(cfg.Librarian.Socket); socket != "" {
		pippi, pippiDialErr = libclient.New(socket, "")
		if pippiDialErr != nil {
			slog.Error("pippi-librarian socket not reachable — sync will fail until librarian starts", "socket", socket, "error", pippiDialErr)
			// Don't fail startup — wiki works without librarian, sync degrades gracefully
		}
		if pippi != nil && pippiDialErr == nil {
			slog.Info("pippi-librarian connected — sync and memory_search enabled", "socket", socket)
		} else if pippi != nil {
			slog.Warn("pippi-librarian socket unreachable at startup — search will use SQLite FTS until librarian is up", "socket", socket, "error", pippiDialErr)
		}
	}
	var libOpts []librarian.Option
	if pippi != nil {
		libOpts = append(libOpts, librarian.WithPippiLibrarian(pippi, treeHolder))
	}

	repoKeyToProjectUUID := make(map[string]string)
	for _, p := range treeIdx.Projects {
		if p == nil || p.RepoKey == "" || p.UUID == "" {
			continue
		}
		if _, ok := repoKeyToProjectUUID[p.RepoKey]; !ok {
			repoKeyToProjectUUID[p.RepoKey] = p.UUID
		}
	}

	librarians := make(map[string]*librarian.Librarian)
	for _, project := range projectKeys {
		vocab := vocabulary.NewStore(db)
		opts := append([]librarian.Option{}, libOpts...)
		if u := repoKeyToProjectUUID[project]; u != "" {
			opts = append(opts, librarian.WithProjectUUID(u))
		}
		librarians[project] = librarian.New(repos[project], indexer, vocab, opts...)
	}

	blobStore := blob.NewStore(dataAbs)

	migrationAliases, err := wgit.LoadMigrationAliases(dataAbs)
	if err != nil {
		slog.Warn("migration-map.json not loaded; page history may miss pre-migration commits", "error", err)
		migrationAliases = nil
	}

	// Create web handler first (builds edit caches), then API server shares the caches.
	webHandler := web.NewHandler(repos, db, librarians, treeHolder, keyStore, migrationAliases)
	apiSrv := api.NewServer(api.ServerConfig{
		Addr:             cfg.Server.Addr,
		Repos:            repos,
		DB:               db,
		Librarians:       librarians,
		Edits:            webHandler.EditCaches(),
		Tree:             treeHolder,
		Blob:             blobStore,
		Keys:             keyStore,
		MigrationAliases: migrationAliases,
	})

	// Compose both into a single root mux.
	mux := http.NewServeMux()

	// Health endpoint — registered first, fast, no middleware.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	mux.Handle("/api/", apiSrv.Handler())
	mux.HandleFunc("GET /blobs/{hash}", api.GetBlob(blobStore))
	webHandler.Register(mux)

	treeWrapped := webHandler.WithTreeRouter(treeAbs, treeHolder, mux)

	// Wrap with middleware.
	handler := api.RequestLogger(api.RateLimiter(100, 200)(treeWrapped))

	return &App{
		Config:     cfg,
		Repos:      repos,
		DB:         db,
		Indexer:    indexer,
		Searcher:   index.NewSearcher(db),
		Librarians: librarians,
		Keys:       keyStore,
		Handler:    handler,
	}, nil
}

// Close releases resources.
func (a *App) Close() {
	if a.DB != nil {
		a.DB.Close()
	}
}

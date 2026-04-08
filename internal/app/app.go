package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/frodex/prd2wiki/internal/api"
	"github.com/frodex/prd2wiki/internal/embedder"
	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/vectordb"
	"github.com/frodex/prd2wiki/internal/vocabulary"
	"github.com/frodex/prd2wiki/internal/web"
)

// Config holds all application configuration.
type Config struct {
	Server   ServerConfig            `yaml:"server"`
	Data     DataConfig              `yaml:"data"`
	Embedder embedder.EmbedderConfig `yaml:"embedder"`
	Projects []string                `yaml:"projects"`
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
	VStore     *vectordb.Store
	Librarians map[string]*librarian.Librarian
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
	if len(cfg.Projects) == 0 {
		cfg.Projects = []string{"default"}
	}

	// Create data directory if needed.
	if err := os.MkdirAll(cfg.Data.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir %q: %w", cfg.Data.Dir, err)
	}

	// Open or initialize git bare repos for each project.
	repos := make(map[string]*wgit.Repo, len(cfg.Projects))
	for _, project := range cfg.Projects {
		repo, err := wgit.OpenRepo(cfg.Data.Dir, project)
		if err != nil {
			// Repo doesn't exist yet -- initialize it.
			repo, err = wgit.InitRepo(cfg.Data.Dir, project)
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
	dbPath := fmt.Sprintf("%s/index.db", cfg.Data.Dir)
	db, err := index.OpenDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", dbPath, err)
	}

	// Rebuild index from repos on startup.
	indexer := index.NewIndexer(db)
	for _, project := range cfg.Projects {
		repo := repos[project]
		branches, err := repo.ListBranches()
		if err != nil {
			slog.Warn("list branches failed, skipping rebuild", "project", project, "error", err)
			continue
		}
		for _, branch := range branches {
			slog.Info("rebuilding index", "project", project, "branch", branch)
			if err := indexer.RebuildFromRepo(project, repo, branch); err != nil {
				slog.Warn("rebuild failed", "project", project, "branch", branch, "error", err)
			}
		}
	}

	// Apply embedder config defaults.
	embCfg := cfg.Embedder
	if embCfg.Endpoint == "" {
		embCfg.Endpoint = os.Getenv("PRDWIKI_EMBEDDER_URL")
	}
	if embCfg.Endpoint == "" {
		embCfg.Endpoint = "http://localhost:8081"
	}
	if embCfg.Dimensions == 0 {
		embCfg.Dimensions = 768
	}
	if embCfg.TimeoutStr == "" {
		embCfg.TimeoutStr = "30s"
	}
	if embCfg.Type == "" {
		embCfg.Type = "llama_cpp"
	}
	if embCfg.QueryPrefix == "" {
		embCfg.QueryPrefix = "search_query: "
	}
	if embCfg.PassagePrefix == "" {
		embCfg.PassagePrefix = "search_document: "
	}

	// Create embedder -- try real LlamaCpp, fall back to Noop.
	var emb embedder.Embedder
	llamaEmb := embedder.NewLlamaCppEmbedder(embCfg)
	if err := llamaEmb.HealthCheck(context.Background()); err == nil {
		emb = llamaEmb
		slog.Info("embedder connected", "type", "llama_cpp", "endpoint", embCfg.Endpoint, "dims", embCfg.Dimensions)
	} else {
		emb = embedder.NoopEmbedder{}
		slog.Warn("embedder unavailable, using noop", "endpoint", embCfg.Endpoint, "error", err)
	}
	vstore := vectordb.NewStore(emb)

	// Load persisted vector index from disk (avoids re-embedding on restart).
	vectorPath := filepath.Join(cfg.Data.Dir, "vectors", "pages.json")
	if err := vstore.LoadFromDisk(vectorPath); err != nil {
		slog.Info("vector index: no persisted data, will embed on first write", "path", vectorPath)
	} else {
		slog.Info("vector index loaded from disk", "entries", vstore.Count(), "path", vectorPath)
	}
	// Enable auto-save so every IndexPage/RemovePage persists to disk.
	vstore.SetPersistPath(vectorPath)

	// Create embedding profile store.
	profileStore, err := embedder.NewEmbeddingProfileStore(db)
	if err != nil {
		// Close db before returning since caller won't have an App to call Close on.
		db.Close()
		return nil, fmt.Errorf("create embedding profile store: %w", err)
	}
	profile := embedder.ProfileFromConfig(embCfg)
	if existing, err := profileStore.Get(context.Background(), profile.ProfileID); err != nil || existing == nil {
		if regErr := profileStore.Register(context.Background(), profile); regErr != nil {
			slog.Warn("register embedding profile failed", "error", regErr)
		}
	}
	_ = profileStore // available for future use

	librarians := make(map[string]*librarian.Librarian)
	for _, project := range cfg.Projects {
		vocab := vocabulary.NewStore(db)
		librarians[project] = librarian.New(repos[project], indexer, vstore, vocab)
	}

	// Rebuild vector index only if nothing was loaded from disk.
	if vstore.Count() == 0 {
		slog.Info("vector index empty, rebuilding from git")
		for _, project := range cfg.Projects {
			lib := librarians[project]
			repo := repos[project]
			branches, _ := repo.ListBranches()
			for _, branch := range branches {
				ctx := context.Background()
				n, err := lib.RebuildVectorIndex(ctx, project, branch)
				if err != nil {
					slog.Warn("vector rebuild failed", "project", project, "branch", branch, "error", err)
				} else if n > 0 {
					slog.Info("vector index rebuilt", "project", project, "branch", branch, "pages", n)
				}
			}
		}
		if vstore.Count() > 0 {
			slog.Info("vector index rebuild complete", "entries", vstore.Count())
		}
	} else {
		slog.Info("vector index loaded, skipping rebuild", "entries", vstore.Count())
	}

	// Create API server and web handler.
	apiSrv := api.NewServer(cfg.Server.Addr, repos, db, librarians)
	webHandler := web.NewHandler(repos, db, librarians)

	// Compose both into a single root mux.
	mux := http.NewServeMux()
	mux.Handle("/api/", apiSrv.Handler())
	webHandler.Register(mux)

	// Wrap with middleware.
	handler := api.RequestLogger(mux)

	return &App{
		Config:     cfg,
		Repos:      repos,
		DB:         db,
		Indexer:    indexer,
		Searcher:   index.NewSearcher(db),
		VStore:     vstore,
		Librarians: librarians,
		Handler:    handler,
	}, nil
}

// Close releases resources.
func (a *App) Close() {
	if a.DB != nil {
		a.DB.Close()
	}
}

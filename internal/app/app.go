package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/frodex/prd2wiki/internal/api"
	"github.com/frodex/prd2wiki/internal/auth"
	"github.com/frodex/prd2wiki/internal/blob"
	"github.com/frodex/prd2wiki/internal/embedder"
	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/libclient"
	"github.com/frodex/prd2wiki/internal/tree"
	"github.com/frodex/prd2wiki/internal/vectordb"
	"github.com/frodex/prd2wiki/internal/vocabulary"
	"github.com/frodex/prd2wiki/internal/web"
)

// Config holds all application configuration.
type Config struct {
	Server    ServerConfig            `yaml:"server"`
	Data      DataConfig              `yaml:"data"`
	Tree      TreeConfig              `yaml:"tree"`
	Librarian LibrarianConfig         `yaml:"librarian"`
	Embedder  embedder.EmbedderConfig `yaml:"embedder"`
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
	VStore     *vectordb.Store
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
		embCfg.Type = "openai"
	}
	if embCfg.QueryPrefix == "" {
		embCfg.QueryPrefix = "search_query: "
	}
	if embCfg.PassagePrefix == "" {
		embCfg.PassagePrefix = "search_document: "
	}

	// Create embedder -- try real LlamaCpp, fall back to Noop.
	var emb embedder.Embedder
	openaiEmb := embedder.NewOpenAIEmbedder(embCfg)
	if err := openaiEmb.HealthCheck(context.Background()); err == nil {
		emb = openaiEmb
		slog.Info("embedder connected", "type", "openai", "endpoint", embCfg.Endpoint, "dims", embCfg.Dimensions)
	} else {
		emb = embedder.NoopEmbedder{}
		slog.Warn("embedder unavailable, using noop", "endpoint", embCfg.Endpoint, "error", err)
	}
	vstore := vectordb.NewStore(emb)

	// Load persisted vector index from disk (avoids re-embedding on restart).
	vectorPath := filepath.Join(dataAbs, "vectors", "pages.json")
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

	var pippi *libclient.Client
	if socket := strings.TrimSpace(cfg.Librarian.Socket); socket != "" {
		pippi = libclient.New(socket, "")
		if pippi != nil {
			slog.Info("pippi-librarian sync enabled", "socket", socket)
		}
	}
	var libOpts []librarian.Option
	if pippi != nil {
		libOpts = append(libOpts, librarian.WithPippiLibrarian(pippi, treeHolder))
	}

	librarians := make(map[string]*librarian.Librarian)
	for _, project := range projectKeys {
		vocab := vocabulary.NewStore(db)
		librarians[project] = librarian.New(repos[project], indexer, vstore, vocab, libOpts...)
	}

	// Rebuild vector index only if nothing was loaded from disk (parallel via errgroup).
	if vstore.Count() == 0 {
		slog.Info("vector index empty, rebuilding from git")
		vg, vctx := errgroup.WithContext(context.Background())
		for _, project := range projectKeys {
			project := project // capture
			lib := librarians[project]
			repo := repos[project]
			branches, _ := repo.ListBranches()
			for _, branch := range branches {
				branch := branch
				vg.Go(func() error {
					n, err := lib.RebuildVectorIndex(vctx, project, branch)
					if err != nil {
						return fmt.Errorf("vector rebuild %s/%s: %w", project, branch, err)
					}
					if n > 0 {
						slog.Info("vector index rebuilt", "project", project, "branch", branch, "pages", n)
					}
					return nil
				})
			}
		}
		if err := vg.Wait(); err != nil {
			slog.Warn("vector index rebuild had errors", "error", err)
		}
		if vstore.Count() > 0 {
			slog.Info("vector index rebuild complete", "entries", vstore.Count())
		}
	} else {
		slog.Info("vector index loaded, skipping rebuild", "entries", vstore.Count())
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
		Addr:       cfg.Server.Addr,
		Repos:      repos,
		DB:         db,
		Librarians: librarians,
		Edits:      webHandler.EditCaches(),
		Tree:       treeHolder,
		Blob:       blobStore,
		Keys:       keyStore,
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
		VStore:     vstore,
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

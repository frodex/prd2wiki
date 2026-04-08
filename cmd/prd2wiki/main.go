package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/frodex/prd2wiki/internal/api"
	"github.com/frodex/prd2wiki/internal/embedder"
	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
	"github.com/frodex/prd2wiki/internal/librarian"
	"github.com/frodex/prd2wiki/internal/mcp"
	"github.com/frodex/prd2wiki/internal/steward"
	"github.com/frodex/prd2wiki/internal/vectordb"
	"github.com/frodex/prd2wiki/internal/vocabulary"
	"github.com/frodex/prd2wiki/internal/web"
)

// Config holds the application configuration loaded from YAML.
type Config struct {
	Server struct {
		Addr string `yaml:"addr"`
	} `yaml:"server"`
	Data struct {
		Dir string `yaml:"dir"`
	} `yaml:"data"`
	Embedder embedder.EmbedderConfig `yaml:"embedder"`
	Projects []string `yaml:"projects"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "steward" {
		runSteward(os.Args[2:])
		return
	}

	configPath := flag.String("config", "config/prd2wiki.yaml", "path to config file")
	flag.Parse()

	// Load config.
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

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
		log.Fatalf("create data dir %q: %v", cfg.Data.Dir, err)
	}

	// Open or initialize git bare repos for each project.
	repos := make(map[string]*wgit.Repo, len(cfg.Projects))
	for _, project := range cfg.Projects {
		repo, err := wgit.OpenRepo(cfg.Data.Dir, project)
		if err != nil {
			// Repo doesn't exist yet — initialize it.
			repo, err = wgit.InitRepo(cfg.Data.Dir, project)
			if err != nil {
				log.Fatalf("init repo for project %q: %v", project, err)
			}
			log.Printf("initialized new repo for project %q", project)
		} else {
			log.Printf("opened existing repo for project %q", project)
		}
		repos[project] = repo
	}

	// Open SQLite index database.
	dbPath := fmt.Sprintf("%s/index.db", cfg.Data.Dir)
	db, err := index.OpenDatabase(dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Rebuild index from repos on startup.
	indexer := index.NewIndexer(db)
	for _, project := range cfg.Projects {
		repo := repos[project]
		branches, err := repo.ListBranches()
		if err != nil {
			log.Printf("warning: list branches for %q: %v (skipping rebuild)", project, err)
			continue
		}
		for _, branch := range branches {
			log.Printf("rebuilding index for %s/%s", project, branch)
			if err := indexer.RebuildFromRepo(project, repo, branch); err != nil {
				log.Printf("warning: rebuild %s/%s: %v", project, branch, err)
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

	// Create embedder — try real LlamaCpp, fall back to Noop.
	var emb embedder.Embedder
	llamaEmb := embedder.NewLlamaCppEmbedder(embCfg)
	if err := llamaEmb.HealthCheck(context.Background()); err == nil {
		emb = llamaEmb
		log.Printf("embedder: connected to LlamaCpp at %s (dims=%d)", embCfg.Endpoint, embCfg.Dimensions)
	} else {
		emb = embedder.NoopEmbedder{}
		log.Printf("embedder: LlamaCpp not available at %s — using noop (lexical search only)", embCfg.Endpoint)
	}
	vstore := vectordb.NewStore(emb)

	// Load persisted vector index from disk (avoids re-embedding on restart).
	vectorPath := filepath.Join(cfg.Data.Dir, "vectors", "pages.json")
	if err := vstore.LoadFromDisk(vectorPath); err != nil {
		log.Printf("vector index: no persisted data at %s, will embed on first write", vectorPath)
	} else {
		log.Printf("vector index: loaded %d entries from disk", vstore.Count())
	}
	// Enable auto-save so every IndexPage/RemovePage persists to disk.
	vstore.SetPersistPath(vectorPath)

	// Create embedding profile store.
	profileStore, err := embedder.NewEmbeddingProfileStore(db)
	if err != nil {
		log.Fatalf("create embedding profile store: %v", err)
	}
	profile := embedder.ProfileFromConfig(embCfg)
	if existing, err := profileStore.Get(context.Background(), profile.ProfileID); err != nil || existing == nil {
		if regErr := profileStore.Register(context.Background(), profile); regErr != nil {
			log.Printf("warning: register embedding profile: %v", regErr)
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
		log.Printf("vector index: empty after disk load, rebuilding from git...")
		for _, project := range cfg.Projects {
			lib := librarians[project]
			repo := repos[project]
			branches, _ := repo.ListBranches()
			for _, branch := range branches {
				ctx := context.Background()
				n, err := lib.RebuildVectorIndex(ctx, project, branch)
				if err != nil {
					log.Printf("warning: vector rebuild %s/%s: %v", project, branch, err)
				} else if n > 0 {
					log.Printf("vector index: embedded %d pages from %s/%s", n, project, branch)
				}
			}
		}
		if vstore.Count() > 0 {
			log.Printf("vector index: rebuild complete, %d entries persisted to disk", vstore.Count())
		}
	} else {
		log.Printf("vector index: %d entries loaded, skipping rebuild", vstore.Count())
	}

	// Create API server and web handler.
	srv := api.NewServer(cfg.Server.Addr, repos, db, librarians)
	webHandler := web.NewHandler(repos, db, librarians)

	// Compose both into a single root mux.
	mux := http.NewServeMux()
	mux.Handle("/api/", srv.Handler())
	webHandler.Register(mux)

	// Start server.
	log.Printf("prd2wiki listening on %s", cfg.Server.Addr)
	log.Printf("  Web UI: http://localhost%s/", cfg.Server.Addr)
	log.Printf("  API:    http://localhost%s/api/", cfg.Server.Addr)
	if err := http.ListenAndServe(cfg.Server.Addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func runSteward(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: prd2wiki steward <lint|resolve|ingest|all> [--project PROJECT]")
		os.Exit(1)
	}

	cmd := args[0]
	project := "default"

	// Parse --project flag.
	for i, a := range args {
		if a == "--project" && i+1 < len(args) {
			project = args[i+1]
		}
	}

	apiURL := os.Getenv("PRDWIKI_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	client := mcp.NewWikiClient(apiURL)

	var reports []*steward.Report

	switch cmd {
	case "lint":
		s := steward.NewLintSteward(client)
		r, err := s.Run(project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lint steward error: %v\n", err)
			os.Exit(1)
		}
		reports = append(reports, r)

	case "resolve":
		s := steward.NewResolveSteward(client)
		r, err := s.Run(project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve steward error: %v\n", err)
			os.Exit(1)
		}
		reports = append(reports, r)

	case "ingest":
		s := steward.NewIngestSteward(client)
		r, err := s.Run(project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ingest steward error: %v\n", err)
			os.Exit(1)
		}
		reports = append(reports, r)

	case "all":
		lintSteward := steward.NewLintSteward(client)
		r, err := lintSteward.Run(project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lint steward error: %v\n", err)
			os.Exit(1)
		}
		reports = append(reports, r)

		resolveSteward := steward.NewResolveSteward(client)
		r, err = resolveSteward.Run(project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve steward error: %v\n", err)
			os.Exit(1)
		}
		reports = append(reports, r)

		ingestSteward := steward.NewIngestSteward(client)
		r, err = ingestSteward.Run(project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ingest steward error: %v\n", err)
			os.Exit(1)
		}
		reports = append(reports, r)

	default:
		fmt.Fprintf(os.Stderr, "unknown steward command: %s\n", cmd)
		os.Exit(1)
	}

	hasErrors := false
	for _, r := range reports {
		data, _ := r.JSON()
		fmt.Println(string(data))
		if r.HasErrors() {
			hasErrors = true
		}
	}

	if hasErrors {
		os.Exit(1)
	}
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %q: %w", path, err)
	}
	return &cfg, nil
}

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/frodex/prd2wiki/internal/api"
	wgit "github.com/frodex/prd2wiki/internal/git"
	"github.com/frodex/prd2wiki/internal/index"
)

// Config holds the application configuration loaded from YAML.
type Config struct {
	Server struct {
		Addr string `yaml:"addr"`
	} `yaml:"server"`
	Data struct {
		Dir string `yaml:"dir"`
	} `yaml:"data"`
	Projects []string `yaml:"projects"`
}

func main() {
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

	// Start the API server.
	srv := api.NewServer(cfg.Server.Addr, repos, db)
	log.Printf("prd2wiki listening on %s", cfg.Server.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
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

package main

import (
	"log/slog"
	"os"

	mcppkg "github.com/frodex/prd2wiki/internal/mcp"
	"github.com/frodex/prd2wiki/internal/tree"
)

func main() {
	// Logs go to stderr; MCP protocol goes to stdout.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	apiURL := os.Getenv("PRDWIKI_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	treeRoot := os.Getenv("PRDWIKI_TREE_ROOT")
	dataDir := os.Getenv("PRDWIKI_DATA_DIR")
	var treeHolder *tree.IndexHolder
	if treeRoot != "" && dataDir != "" {
		idx, err := tree.Scan(treeRoot, dataDir)
		if err != nil {
			slog.Error("tree scan failed", "error", err, "tree_root", treeRoot, "data_dir", dataDir)
			os.Exit(1)
		}
		treeHolder = tree.NewIndexHolder(treeRoot, dataDir, idx)
		slog.Info("prd2wiki-mcp tree index loaded", "tree_root", treeRoot, "data_dir", dataDir)
	} else {
		slog.Info("prd2wiki-mcp tree not configured (PRDWIKI_TREE_ROOT / PRDWIKI_DATA_DIR empty); tree-path tools limited")
	}

	client := mcppkg.NewWikiClient(apiURL)
	srv := mcppkg.NewServer(mcppkg.ServerConfig{Client: client, TreeHolder: treeHolder})

	slog.Info("prd2wiki-mcp starting", "api_url", apiURL)
	slog.Info("prd2wiki-mcp reading from stdin, writing to stdout")

	srv.ServeStdio()

	slog.Info("prd2wiki-mcp exiting (stdin closed or EOF)")
}

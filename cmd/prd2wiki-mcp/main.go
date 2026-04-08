package main

import (
	"log/slog"
	"os"

	mcppkg "github.com/frodex/prd2wiki/internal/mcp"
)

func main() {
	// Logs go to stderr; MCP protocol goes to stdout.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	apiURL := os.Getenv("PRDWIKI_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	client := mcppkg.NewWikiClient(apiURL)
	srv := mcppkg.NewServer(client)

	slog.Info("prd2wiki-mcp starting", "api_url", apiURL)
	slog.Info("prd2wiki-mcp reading from stdin, writing to stdout")

	srv.ServeStdio()

	slog.Info("prd2wiki-mcp exiting (stdin closed or EOF)")
}

package main

import (
	"log"
	"os"

	mcppkg "github.com/frodex/prd2wiki/internal/mcp"
)

func main() {
	apiURL := os.Getenv("PRDWIKI_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	client := mcppkg.NewWikiClient(apiURL)
	srv := mcppkg.NewServer(client)

	log.SetOutput(os.Stderr) // logs go to stderr, MCP protocol goes to stdout
	log.Printf("prd2wiki-mcp starting, wiki API: %s", apiURL)

	srv.ServeStdio()
}

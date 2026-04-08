package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/frodex/prd2wiki/internal/app"
	"github.com/frodex/prd2wiki/internal/mcp"
	"github.com/frodex/prd2wiki/internal/steward"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "steward" {
		runSteward(os.Args[2:])
		return
	}

	configPath := flag.String("config", "config/prd2wiki.yaml", "config file")
	flag.Parse()

	slog.Info("prd2wiki starting", "config", *configPath)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	application, err := app.New(*cfg)
	if err != nil {
		slog.Error("initialize app", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      application.Handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		slog.Info("shutting down...")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			slog.Error("shutdown error", "error", err)
		}
	}()

	slog.Info("prd2wiki listening", "addr", cfg.Server.Addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped cleanly")
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

func loadConfig(path string) (*app.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	var cfg app.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %q: %w", path, err)
	}
	return &cfg, nil
}

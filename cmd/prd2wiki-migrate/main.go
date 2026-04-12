// prd2wiki-migrate: history-preserving page migration from old hash-prefix IDs to UUID paths.
//
// Usage:
//
//	prd2wiki-migrate --data ./data --tree ./tree --plan          # dry run — build plan, no changes
//	prd2wiki-migrate --data ./data --tree ./tree --execute       # run migration
//	prd2wiki-migrate --data ./data --tree ./tree --plan-file migration-plan.json --execute  # use saved plan
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/frodex/prd2wiki/internal/migrate"
)

func main() {
	dataDir := flag.String("data", "./data", "path to data directory")
	treeDir := flag.String("tree", "./tree", "path to tree directory (created by migration)")
	planOnly := flag.Bool("plan", false, "build and print plan, do not execute")
	execute := flag.Bool("execute", false, "execute migration")
	planFile := flag.String("plan-file", "", "save/load plan to/from this file")
	flag.Parse()

	if !*planOnly && !*execute {
		fmt.Fprintln(os.Stderr, "specify --plan (dry run) or --execute (run migration)")
		flag.Usage()
		os.Exit(1)
	}

	// Project configuration — customize for your wiki
	projects := []migrate.ProjectConfig{
		{OldName: "default", TreePath: "prd2wiki", DisplayName: "PRD Wiki"},
		{OldName: "svg-terminal", TreePath: "svg-terminal", DisplayName: "SVG Terminal"},
		{OldName: "battletech", TreePath: "games/battletech", DisplayName: "BattleTech"},
		{OldName: "phat-toad-with-trails", TreePath: "phat-toad", DisplayName: "PHAT-TOAD"},
	}

	var plan *migrate.Plan
	var err error

	// Load existing plan or build new one
	if *planFile != "" && *execute {
		// Try to load existing plan
		plan, err = migrate.LoadPlan(*planFile)
		if err != nil {
			slog.Error("could not load plan", "file", *planFile, "error", err)
			os.Exit(1)
		}
		slog.Info("loaded plan", "file", *planFile, "pages", len(plan.Pages))
	} else {
		// Build plan by scanning repos
		slog.Info("building migration plan", "data", *dataDir)
		plan, err = migrate.BuildPlan(*dataDir, *treeDir, projects)
		if err != nil {
			slog.Error("plan failed", "error", err)
			os.Exit(1)
		}
		slog.Info("plan built", "pages", len(plan.Pages), "projects", len(plan.Projects))
	}

	// Save plan if requested
	if *planFile != "" && *planOnly {
		if err := migrate.SavePlan(plan, *planFile); err != nil {
			slog.Error("save plan failed", "error", err)
			os.Exit(1)
		}
		slog.Info("plan saved", "file", *planFile)
	}

	if *planOnly {
		// Print summary
		fmt.Printf("Migration plan: %d pages across %d projects\n\n", len(plan.Pages), len(plan.Projects))
		for _, proj := range plan.Projects {
			fmt.Printf("Project: %s → %s (%s)\n", proj.OldName, proj.TreePath, proj.DisplayName)
		}
		fmt.Println()
		for oldID, page := range plan.Pages {
			fmt.Printf("  %s → %s  %s\n    %s → %s\n    created: %s  slug: %s\n\n",
				oldID, page.UUID, page.Title,
				page.OldPath, page.NewPath,
				page.FirstCommit.Format("2006-01-02"), page.Slug)
		}
		return
	}

	if *execute {
		slog.Info("executing migration")
		if err := migrate.Execute(plan); err != nil {
			slog.Error("migration failed", "error", err)
			os.Exit(1)
		}

		// Save the plan as migration-map for future reference
		mapPath := *dataDir + "/migration-map.json"
		if err := migrate.SavePlan(plan, mapPath); err != nil {
			slog.Warn("could not save migration map", "error", err)
		} else {
			slog.Info("migration map saved", "path", mapPath)
		}

		slog.Info("migration complete",
			"pages", len(plan.Pages),
			"projects", len(plan.Projects),
			"tree", *treeDir)
	}
}

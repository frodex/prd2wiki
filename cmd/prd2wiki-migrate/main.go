// prd2wiki-migrate: history-preserving page migration from old hash-prefix IDs to UUID paths.
//
// Usage:
//
//	prd2wiki-migrate --data ./data --tree ./tree --plan                     # dry run
//	prd2wiki-migrate --data ./data --tree ./tree --execute                  # migrate
//	prd2wiki-migrate --data ./data --tree ./tree --verify                   # check results
//	prd2wiki-migrate --data ./data --tree ./tree --plan-file plan.json --execute  # use saved plan
//	prd2wiki-migrate --data ./data --tree ./tree --projects-file projects.json    # custom project config
package main

import (
	"encoding/json"
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
	verify := flag.Bool("verify", false, "verify migration results (run after --execute)")
	planFile := flag.String("plan-file", "", "save/load plan to/from this file")
	projectsFile := flag.String("projects-file", "", "JSON file with project config (default: built-in)")
	flag.Parse()

	if !*planOnly && !*execute && !*verify {
		fmt.Fprintln(os.Stderr, "specify --plan (dry run), --execute (migrate), or --verify (check)")
		flag.Usage()
		os.Exit(1)
	}

	projects := defaultProjects()
	if *projectsFile != "" {
		var err error
		projects, err = loadProjectsFile(*projectsFile)
		if err != nil {
			slog.Error("load projects file", "file", *projectsFile, "error", err)
			os.Exit(1)
		}
	}

	var plan *migrate.Plan
	var err error

	// Load existing plan or build new one
	if *planFile != "" && (*execute || *verify) {
		plan, err = migrate.LoadPlan(*planFile)
		if err != nil {
			slog.Error("could not load plan", "file", *planFile, "error", err)
			os.Exit(1)
		}
		slog.Info("loaded plan", "file", *planFile, "pages", len(plan.Pages))
	} else {
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
		printPlan(plan)
		return
	}

	if *execute {
		slog.Info("executing migration")
		if err := migrate.Execute(plan); err != nil {
			slog.Error("migration failed", "error", err)
			os.Exit(1)
		}

		// Save migration map
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

		// Auto-verify after execute
		slog.Info("running post-migration verification")
		result, err := migrate.Verify(plan)
		if err != nil {
			slog.Error("verification error", "error", err)
			os.Exit(1)
		}
		printVerifyResult(result)
		if !result.OK() {
			os.Exit(1)
		}
		return
	}

	if *verify {
		// Verify only — plan must have been saved or loaded
		slog.Info("verifying migration")
		result, err := migrate.Verify(plan)
		if err != nil {
			slog.Error("verification error", "error", err)
			os.Exit(1)
		}
		printVerifyResult(result)
		if !result.OK() {
			os.Exit(1)
		}
	}
}

func printPlan(plan *migrate.Plan) {
	fmt.Printf("Migration plan: %d pages across %d projects\n\n", len(plan.Pages), len(plan.Projects))
	for _, proj := range plan.Projects {
		fmt.Printf("Project: %s → %s (%s)\n", proj.OldName, proj.TreePath, proj.DisplayName)
	}
	fmt.Println()

	i := 0
	for oldID, page := range plan.Pages {
		i++
		fmt.Printf("  [%d/%d] %s → %s\n    %s\n    %s → %s\n    created: %s  slug: %s\n\n",
			i, len(plan.Pages),
			oldID, page.UUID, page.Title,
			page.OldPath, page.NewPath,
			page.FirstCommit.Format("2006-01-02"), page.Slug)
	}
}

func printVerifyResult(r *migrate.VerifyResult) {
	fmt.Printf("\nVerification: %d pages checked\n", r.Total)
	fmt.Printf("  History OK:    %d\n", r.HistoryOK)
	fmt.Printf("  History bad:   %d\n", r.HistoryBad)
	fmt.Printf("  CrossRefs bad: %d\n", r.CrossRefsBad)
	if len(r.Errors) > 0 {
		fmt.Printf("\nErrors (%d):\n", len(r.Errors))
		for _, e := range r.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}
	if r.OK() {
		fmt.Println("\n✓ Migration verified OK")
	} else {
		fmt.Println("\n✗ Migration has issues")
	}
}

func defaultProjects() []migrate.ProjectConfig {
	return []migrate.ProjectConfig{
		{OldName: "default", TreePath: "prd2wiki", DisplayName: "PRD Wiki"},
		{OldName: "svg-terminal", TreePath: "svg-terminal", DisplayName: "SVG Terminal"},
		{OldName: "battletech", TreePath: "games/battletech", DisplayName: "BattleTech"},
		{OldName: "phat-toad-with-trails", TreePath: "phat-toad", DisplayName: "PHAT-TOAD"},
	}
}

func loadProjectsFile(path string) ([]migrate.ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var projects []migrate.ProjectConfig
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

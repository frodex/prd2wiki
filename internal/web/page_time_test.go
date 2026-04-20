package web

import (
	"testing"

	"github.com/frodex/prd2wiki/internal/index"
)

func TestPageUpdatedSortKey_prefersGitOverSQLite(t *testing.T) {
	pr := index.PageResult{
		UpdatedAt: "2026-04-20 12:00:00", // indexer touch after rebuild
	}
	edit := EditInfo{Author: "a", Date: "2026-04-10 15:30"}
	got := PageUpdatedSortKey(pr, edit, true)
	if got == "" || got[0:10] != "2026-04-10" {
		t.Fatalf("want git date 2026-04-10, got %q", got)
	}
}

func TestPageUpdatedDisplay_dcModifiedFallback(t *testing.T) {
	pr := index.PageResult{
		DCModified: "2026-04-10",
		UpdatedAt:  "2026-04-20 00:00:00",
	}
	got := PageUpdatedDisplay(pr, EditInfo{}, false)
	if got != "2026-04-10" {
		t.Fatalf("want dc_modified display, got %q", got)
	}
}

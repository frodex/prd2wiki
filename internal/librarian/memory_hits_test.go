package librarian

import (
	"testing"

	"github.com/frodex/prd2wiki/internal/libclient"
)

func TestAggregateMemorySearchHits_bestHitPerPage(t *testing.T) {
	hits := []libclient.MemorySearchHit{
		{PageUUID: "p1", Score: 0.95, VersionStatus: "superseded", SourceCommit: "deadbeefcafef00d", Snippet: "older"},
		{PageUUID: "p1", Score: 0.5, VersionStatus: "current", Snippet: "newer"},
	}
	out := aggregateMemorySearchHits(hits, 10)
	if len(out) != 1 {
		t.Fatalf("len %d", len(out))
	}
	if !out[0].MatchFromHistory || out[0].HistoryCommit != "deadbeefcafe" {
		t.Fatalf("want history match + 12-char sha, got %+v", out[0])
	}
}

func TestAggregateMemorySearchHits_currentHeadPreferredWhenFirst(t *testing.T) {
	hits := []libclient.MemorySearchHit{
		{PageUUID: "p1", Score: 0.9, VersionStatus: "current", Snippet: "head"},
		{PageUUID: "p1", Score: 0.4, VersionStatus: "superseded", SourceCommit: "abc", Snippet: "tail"},
	}
	out := aggregateMemorySearchHits(hits, 10)
	if len(out) != 1 || out[0].MatchFromHistory {
		t.Fatalf("got %+v", out[0])
	}
}

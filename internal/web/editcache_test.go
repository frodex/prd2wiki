package web

import (
	"testing"
	"time"

	wgit "github.com/frodex/prd2wiki/internal/git"
)

func TestPickLastNonMigrateCommit(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	t1 := time.Date(2025, 6, 1, 9, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		in   []wgit.CommitInfo
		want string // author of chosen
	}{
		{
			name: "skip migrate use prior",
			in: []wgit.CommitInfo{
				{Author: "m", Message: "migrate: x → y (t)", Date: t2},
				{Author: "alice", Message: "real edit", Date: t1},
			},
			want: "alice",
		},
		{
			name: "multiline migrate still skipped",
			in: []wgit.CommitInfo{
				{Author: "m", Message: "migrate: remove old\n\nbody", Date: t2},
				{Author: "bob", Message: "docs: fix", Date: t1},
			},
			want: "bob",
		},
		{
			name: "non-migrate first wins",
			in: []wgit.CommitInfo{
				{Author: "carol", Message: "typo", Date: t2},
			},
			want: "carol",
		},
		{
			name: "all migrate fallback newest",
			in: []wgit.CommitInfo{
				{Author: "m1", Message: "migrate: a", Date: t2},
				{Author: "m2", Message: "migrate: b", Date: t1},
				{Author: "m3", Message: "migrate: c", Date: t0},
			},
			want: "m1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pickLastNonMigrateCommit(tc.in)
			if got.Author != tc.want {
				t.Fatalf("author: got %q want %q", got.Author, tc.want)
			}
		})
	}
}

func TestIsMigrateCommitMessage(t *testing.T) {
	if !isMigrateCommitMessage("migrate: foo") {
		t.Fatal("expected migrate")
	}
	if !isMigrateCommitMessage("  migrate: foo  \nextra") {
		t.Fatal("expected migrate on first line")
	}
	if isMigrateCommitMessage("docs: migrate: not a prefix") {
		t.Fatal("first line must start with migrate:")
	}
}

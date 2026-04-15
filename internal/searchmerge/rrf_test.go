package searchmerge

import (
	"reflect"
	"testing"
)

func TestMergeRRF_empty(t *testing.T) {
	got := MergeRRF(nil, nil, 60)
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestMergeRRF_oneSide(t *testing.T) {
	got := MergeRRF([]string{"a", "b"}, nil, 60)
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("got %v", got)
	}
}

func TestMergeRRF_symmetricTieFavorsFTS(t *testing.T) {
	// Same total RRF for a and b; earlier FTS rank wins the tie.
	fts := []string{"a", "b"}
	vec := []string{"b", "a"}
	got := MergeRRF(fts, vec, 60)
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("got %v, want [a b]", got)
	}
}

func TestMergeRRF_overlapBoostsRank(t *testing.T) {
	// b appears in both lists; a and c only in FTS — b should lead.
	fts := []string{"a", "b", "c"}
	vec := []string{"b"}
	got := MergeRRF(fts, vec, 60)
	if len(got) != 3 || got[0] != "b" {
		t.Fatalf("got %v, want b first", got)
	}
}

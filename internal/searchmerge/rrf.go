package searchmerge

import (
	"math"
	"sort"
)

// DefaultRRFK is the usual RRF smoothing constant (e.g. Cormack et al. style fusion).
const DefaultRRFK = 60

// MergeRRF fuses two ordered hit lists (FTS and vector) by reciprocal rank fusion.
// Higher scores sort first. Ties favor better (lower) FTS rank, then better vector rank.
func MergeRRF(ftsIDs, vecIDs []string, k int) []string {
	if k <= 0 {
		k = DefaultRRFK
	}

	const absent = math.MaxInt32
	type meta struct {
		score  float64
		ftsIdx int
		vecIdx int
	}
	m := make(map[string]*meta)

	get := func(id string) *meta {
		x, ok := m[id]
		if !ok {
			x = &meta{ftsIdx: absent, vecIdx: absent}
			m[id] = x
		}
		return x
	}

	for i, id := range ftsIDs {
		mm := get(id)
		mm.score += 1.0 / (float64(k) + float64(i+1))
		if i < mm.ftsIdx {
			mm.ftsIdx = i
		}
	}
	for i, id := range vecIDs {
		mm := get(id)
		mm.score += 1.0 / (float64(k) + float64(i+1))
		if i < mm.vecIdx {
			mm.vecIdx = i
		}
	}

	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		ai, aj := m[ids[i]], m[ids[j]]
		if ai.score != aj.score {
			return ai.score > aj.score
		}
		if ai.ftsIdx != aj.ftsIdx {
			return ai.ftsIdx < aj.ftsIdx
		}
		return ai.vecIdx < aj.vecIdx
	})
	return ids
}

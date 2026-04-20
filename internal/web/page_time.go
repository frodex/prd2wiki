package web

import (
	"strings"
	"time"

	"github.com/frodex/prd2wiki/internal/index"
)

// FillPageTimestamps sets LastEdit*, UpdatedAtSort, and UpdatedDisplay from git cache + SQLite.
func FillPageTimestamps(item *PageListItem, pr index.PageResult, cache *EditCache) {
	var edit EditInfo
	has := false
	if cache != nil {
		if info, ok := cache.Get(pr.Path); ok {
			edit, has = info, true
			item.LastEditBy = info.Author
			item.LastEditDate = info.Date
		}
	}
	item.UpdatedAtSort = PageUpdatedSortKey(pr, edit, has)
	item.UpdatedDisplay = PageUpdatedDisplay(pr, edit, has)
}

// PageUpdatedSortKey returns an RFC3339 UTC timestamp for stable chronological sorting (newest last lexicographically when ascending — client uses same string compare).
// Precedence: git last commit (edit cache) > Dublin Core dc_modified > SQLite updated_at.
// We do not take max(git, sqlite): after a full reindex, updated_at is "indexer touch" time and would
// incorrectly swamp real edit times for sorting.
func PageUpdatedSortKey(pr index.PageResult, edit EditInfo, hasEdit bool) string {
	if hasEdit && strings.TrimSpace(edit.Date) != "" {
		if t, ok := parseEditCacheDate(edit.Date); ok {
			return t.UTC().Format(time.RFC3339)
		}
	}
	if t, ok := parseDCModifiedDate(pr.DCModified); ok {
		return t.UTC().Format(time.RFC3339)
	}
	if t, ok := parseSQLiteUpdatedAt(pr.UpdatedAt); ok {
		return t.UTC().Format(time.RFC3339)
	}
	return ""
}

// PageUpdatedDisplay is a short human line for the "updated" column.
func PageUpdatedDisplay(pr index.PageResult, edit EditInfo, hasEdit bool) string {
	if hasEdit && strings.TrimSpace(edit.Date) != "" {
		if strings.TrimSpace(edit.Author) != "" {
			return edit.Author + " · " + edit.Date
		}
		return edit.Date
	}
	if t, ok := parseDCModifiedDate(pr.DCModified); ok {
		return t.UTC().Format("2006-01-02")
	}
	if t, ok := parseSQLiteUpdatedAt(pr.UpdatedAt); ok {
		return t.UTC().Format("2006-01-02 15:04")
	}
	return ""
}

func parseDCModifiedDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func parseEditCacheDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func parseSQLiteUpdatedAt(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

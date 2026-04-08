package schema

import (
	"regexp"
	"testing"
	"time"
)

func TestGeneratePageID(t *testing.T) {
	ts := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)

	id := GeneratePageID("Hello World", ts)

	// Must be 7 hex chars.
	if !regexp.MustCompile(`^[0-9a-f]{7}$`).MatchString(id) {
		t.Errorf("GeneratePageID returned %q, want 7 hex chars", id)
	}
}

func TestGeneratePageIDDeterministic(t *testing.T) {
	ts := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)

	id1 := GeneratePageID("Hello World", ts)
	id2 := GeneratePageID("Hello World", ts)

	if id1 != id2 {
		t.Errorf("same input gave different IDs: %q vs %q", id1, id2)
	}
}

func TestGeneratePageIDUniqueness(t *testing.T) {
	ts := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)

	id1 := GeneratePageID("Hello World", ts)
	id2 := GeneratePageID("Different Title", ts)
	id3 := GeneratePageID("Hello World", ts.Add(time.Nanosecond))

	if id1 == id2 {
		t.Errorf("different titles gave same ID: %q", id1)
	}
	if id1 == id3 {
		t.Errorf("different timestamps gave same ID: %q", id1)
	}
}

func TestIsHashID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"a1b2c3d", true},
		{"0000000", true},
		{"fffffff", true},
		{"design-003", false},
		{"abc", false},
		{"ABCDEFG", false}, // uppercase
		{"a1b2c3g", false}, // 'g' not hex
		{"", false},
	}
	for _, tt := range tests {
		if got := IsHashID(tt.id); got != tt.want {
			t.Errorf("IsHashID(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

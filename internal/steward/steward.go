package steward

import (
	"encoding/json"
	"time"
)

// Finding represents a single issue found during a steward run.
type Finding struct {
	Severity string `json:"severity"`        // error | warning | info
	PageID   string `json:"page_id"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
	Action   string `json:"action,omitempty"` // recommended action
}

// Report is the output of a steward run.
type Report struct {
	Steward   string    `json:"steward"` // lint | resolve | ingest
	Project   string    `json:"project"`
	Timestamp time.Time `json:"timestamp"`
	Findings  []Finding `json:"findings"`
	Summary   string    `json:"summary"`
}

// CountBySeverity returns counts of findings by severity.
func (r *Report) CountBySeverity() map[string]int {
	counts := map[string]int{"error": 0, "warning": 0, "info": 0}
	for _, f := range r.Findings {
		counts[f.Severity]++
	}
	return counts
}

// HasErrors returns true if any finding has error severity.
func (r *Report) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == "error" {
			return true
		}
	}
	return false
}

// JSON returns the report as formatted JSON.
func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

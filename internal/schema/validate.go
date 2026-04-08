package schema

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Issue represents a single validation finding.
type Issue struct {
	Severity string `json:"severity"`
	Field    string `json:"field"`
	Message  string `json:"message"`
}

// validTypes lists all recognised page types.
var validTypes = map[string]bool{
	"requirement": true,
	"concept":     true,
	"task":        true,
	"reference":   true,
	"decision":    true,
	"source":      true,
	"config":      true,
}

// validStatuses lists all recognised page statuses.
var validStatuses = map[string]bool{
	"draft":      true,
	"review":     true,
	"active":     true,
	"contested":  true,
	"stale":      true,
	"superseded": true,
	"deprecated": true,
}

// Validate inspects a Frontmatter value and returns all issues found.
// It collects every applicable issue rather than stopping at the first.
func Validate(fm *Frontmatter) []Issue {
	var issues []Issue

	// --- Errors ---

	// Required: id
	if fm.ID == "" {
		issues = append(issues, Issue{
			Severity: SeverityError,
			Field:    "id",
			Message:  "id is required",
		})
	}

	// Required: title
	if fm.Title == "" {
		issues = append(issues, Issue{
			Severity: SeverityError,
			Field:    "title",
			Message:  "title is required",
		})
	}

	// Required + valid: type
	if fm.Type == "" {
		issues = append(issues, Issue{
			Severity: SeverityError,
			Field:    "type",
			Message:  "type is required",
		})
	} else if !validTypes[fm.Type] {
		issues = append(issues, Issue{
			Severity: SeverityError,
			Field:    "type",
			Message:  "type " + fm.Type + " is not valid; must be one of requirement, concept, task, reference, decision, source, config",
		})
	}

	// Valid (optional): status — if present must be a known value
	if fm.Status != "" && !validStatuses[fm.Status] {
		issues = append(issues, Issue{
			Severity: SeverityError,
			Field:    "status",
			Message:  "status " + fm.Status + " is not valid; must be one of draft, review, active, contested, stale, superseded, deprecated",
		})
	}

	// Range: trust_level must be 0-3
	if fm.TrustLevel < 0 || fm.TrustLevel > 3 {
		issues = append(issues, Issue{
			Severity: SeverityError,
			Field:    "trust_level",
			Message:  "trust_level must be between 0 and 3 inclusive",
		})
	}

	// Conditional required: source_meta required when type=source
	if fm.Type == "source" && fm.SourceMeta == nil {
		issues = append(issues, Issue{
			Severity: SeverityError,
			Field:    "source_meta",
			Message:  "source_meta is required when type is 'source'",
		})
	}

	// --- Warnings ---

	// Recommend at least one provenance source
	if len(fm.Provenance.Sources) == 0 {
		issues = append(issues, Issue{
			Severity: SeverityWarning,
			Field:    "provenance.sources",
			Message:  "no provenance sources listed; traceability may be incomplete",
		})
	}

	// Recommend dc.creator
	if fm.DCCreator == "" {
		issues = append(issues, Issue{
			Severity: SeverityWarning,
			Field:    "dc.creator",
			Message:  "dc.creator is not set; authorship is unknown",
		})
	}

	return issues
}

// HasErrors returns true if any issue in the slice has severity "error".
func HasErrors(issues []Issue) bool {
	for _, iss := range issues {
		if iss.Severity == SeverityError {
			return true
		}
	}
	return false
}

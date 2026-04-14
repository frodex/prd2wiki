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
// An empty type is valid (unclassified page).
var validTypes = map[string]bool{
	"research":  true,
	"spec":      true,
	"plan":      true,
	"report":    true,
	"review":    true,
	"reference": true,
	"tracking":  true,
	"skill":     true,
	"rule":      true,
	"project":   true,
	"_test":     true,
}

// legacyTypes maps old type names to new ones (accepted with warning).
var legacyTypes = map[string]string{
	"requirement": "spec",
	"concept":     "research",
	"task":        "plan",
	"decision":    "reference",
	"source":      "reference",
	"config":      "reference",
	"feedback":    "report",
}

// validStatuses lists all recognised page statuses.
var validStatuses = map[string]bool{
	"draft":       true,
	"submitted":   true,
	"approved":    true,
	"rejected":    true,
	"implemented": true,
	"completed":   true,
	"superseded":  true,
	"retired":     true,
}

// legacyStatuses maps old status names to new ones (accepted with warning).
var legacyStatuses = map[string]string{
	"review":     "submitted",
	"proposed":   "submitted",
	"active":     "approved",
	"contested":  "submitted",
	"stale":      "retired",
	"deprecated": "retired",
	"sketch":     "draft",
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

	// Valid: type — empty is allowed (unclassified page)
	if fm.Type != "" {
		if !validTypes[fm.Type] {
			if newType, ok := legacyTypes[fm.Type]; ok {
				issues = append(issues, Issue{
					Severity: SeverityWarning,
					Field:    "type",
					Message:  "type " + fm.Type + " is deprecated; use " + newType + " instead",
				})
			} else {
				issues = append(issues, Issue{
					Severity: SeverityError,
					Field:    "type",
					Message:  "type " + fm.Type + " is not valid; must be one of research, spec, plan, report, review, reference, tracking, skill, rule, project, _test (or empty for unclassified)",
				})
			}
		}
	}

	// Valid (optional): status — if present must be a known value
	if fm.Status != "" && !validStatuses[fm.Status] {
		if newStatus, ok := legacyStatuses[fm.Status]; ok {
			issues = append(issues, Issue{
				Severity: SeverityWarning,
				Field:    "status",
				Message:  "status " + fm.Status + " is deprecated; use " + newStatus + " instead",
			})
		} else {
			issues = append(issues, Issue{
				Severity: SeverityError,
				Field:    "status",
				Message:  "status " + fm.Status + " is not valid; must be one of draft, submitted, approved, rejected, implemented, completed, superseded, retired",
			})
		}
	}

	// Range: trust_level must be 0-3
	if fm.TrustLevel < 0 || fm.TrustLevel > 3 {
		issues = append(issues, Issue{
			Severity: SeverityError,
			Field:    "trust_level",
			Message:  "trust_level must be between 0 and 3 inclusive",
		})
	}

	// Note: source_meta is optional metadata for any page with external origin.
	// The old type=source requirement has been removed — source is now a legacy
	// type that maps to reference.

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

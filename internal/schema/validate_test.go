package schema

import "testing"

// filterSeverity returns only the issues matching the given severity.
func filterSeverity(issues []Issue, sev string) []Issue {
	var out []Issue
	for _, iss := range issues {
		if iss.Severity == sev {
			out = append(out, iss)
		}
	}
	return out
}

// TestValidateSpec checks that a valid minimal spec produces no errors.
func TestValidateSpec(t *testing.T) {
	fm := &Frontmatter{
		ID:     "REQ-001",
		Title:  "My spec",
		Type:   "spec",
		Status: "draft",
		Provenance: Provenance{
			Sources: []Source{
				{Ref: "prd-v1"},
			},
		},
		DCCreator: "alice",
	}
	issues := Validate(fm)
	errs := filterSeverity(issues, SeverityError)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %d: %v", len(errs), errs)
	}
}

// TestValidateEmptyType checks that an empty type is valid (unclassified page).
func TestValidateEmptyType(t *testing.T) {
	fm := &Frontmatter{
		ID:    "PAGE-001",
		Title: "Unclassified page",
		Type:  "",
	}
	issues := Validate(fm)
	errs := filterSeverity(issues, SeverityError)
	for _, e := range errs {
		if e.Field == "type" {
			t.Errorf("empty type should not produce an error, got: %v", e)
		}
	}
}

// TestValidateLegacyType checks that old type names produce a warning, not an error.
func TestValidateLegacyType(t *testing.T) {
	fm := &Frontmatter{
		ID:    "REQ-002",
		Title: "Legacy type",
		Type:  "requirement",
	}
	issues := Validate(fm)
	errs := filterSeverity(issues, SeverityError)
	for _, e := range errs {
		if e.Field == "type" {
			t.Errorf("legacy type should not produce an error, got: %v", e)
		}
	}
	warnings := filterSeverity(issues, SeverityWarning)
	found := false
	for _, w := range warnings {
		if w.Field == "type" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected deprecation warning on legacy type 'requirement'")
	}
}

// TestValidateNewTypes checks that all new types are accepted.
func TestValidateNewTypes(t *testing.T) {
	types := []string{"research", "spec", "plan", "report", "review", "reference", "tracking", "skill", "rule", "project", "_test"}
	for _, typ := range types {
		fm := &Frontmatter{
			ID:    "T-001",
			Title: "Test " + typ,
			Type:  typ,
		}
		issues := Validate(fm)
		errs := filterSeverity(issues, SeverityError)
		for _, e := range errs {
			if e.Field == "type" {
				t.Errorf("type %q should be valid, got error: %v", typ, e)
			}
		}
	}
}

// TestValidateNewStatuses checks that all new statuses are accepted.
func TestValidateNewStatuses(t *testing.T) {
	statuses := []string{"draft", "submitted", "approved", "rejected", "implemented", "completed", "superseded", "retired"}
	for _, status := range statuses {
		fm := &Frontmatter{
			ID:     "S-001",
			Title:  "Test " + status,
			Type:   "spec",
			Status: status,
		}
		issues := Validate(fm)
		errs := filterSeverity(issues, SeverityError)
		for _, e := range errs {
			if e.Field == "status" {
				t.Errorf("status %q should be valid, got error: %v", status, e)
			}
		}
	}
}

// TestValidateLegacyStatus checks that old status names produce a warning, not an error.
func TestValidateLegacyStatus(t *testing.T) {
	for _, old := range []string{"active", "deprecated", "sketch", "stale", "review", "contested", "proposed"} {
		fm := &Frontmatter{
			ID:     "S-002",
			Title:  "Legacy status " + old,
			Type:   "spec",
			Status: old,
		}
		issues := Validate(fm)
		errs := filterSeverity(issues, SeverityError)
		for _, e := range errs {
			if e.Field == "status" {
				t.Errorf("legacy status %q should not produce an error, got: %v", old, e)
			}
		}
		warnings := filterSeverity(issues, SeverityWarning)
		found := false
		for _, w := range warnings {
			if w.Field == "status" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected deprecation warning on legacy status %q", old)
		}
	}
}

// TestValidateRetiredStatus checks that retired is a valid status.
func TestValidateRetiredStatus(t *testing.T) {
	fm := &Frontmatter{
		ID:     "S-003",
		Title:  "Retired page",
		Type:   "reference",
		Status: "retired",
	}
	issues := Validate(fm)
	errs := filterSeverity(issues, SeverityError)
	for _, e := range errs {
		if e.Field == "status" {
			t.Errorf("retired status should not produce an error, got: %v", e)
		}
	}
}

// TestValidateMissingRequired checks that missing id produces an error.
// Note: empty type is now valid (unclassified page), so only id is required.
func TestValidateMissingRequired(t *testing.T) {
	fm := &Frontmatter{
		Title:  "No ID",
		Status: "draft",
	}
	issues := Validate(fm)
	errs := filterSeverity(issues, SeverityError)
	found := false
	for _, e := range errs {
		if e.Field == "id" {
			found = true
		}
	}
	if !found {
		t.Error("expected error on field 'id'")
	}
}

// TestValidateInvalidType checks that an unknown type produces an error on the type field.
func TestValidateInvalidType(t *testing.T) {
	fm := &Frontmatter{
		ID:     "REQ-002",
		Title:  "Bad type",
		Type:   "nonexistent",
		Status: "draft",
	}
	issues := Validate(fm)
	errs := filterSeverity(issues, SeverityError)
	found := false
	for _, e := range errs {
		if e.Field == "type" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error on field 'type', got: %v", errs)
	}
}

// TestValidateInvalidStatus checks that an unknown status produces an error on the status field.
func TestValidateInvalidStatus(t *testing.T) {
	fm := &Frontmatter{
		ID:     "REQ-003",
		Title:  "Bad status",
		Type:   "spec",
		Status: "bogus",
	}
	issues := Validate(fm)
	errs := filterSeverity(issues, SeverityError)
	found := false
	for _, e := range errs {
		if e.Field == "status" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error on field 'status', got: %v", errs)
	}
}

// TestValidateWarningMissingProvenance checks that a valid page without sources produces a warning.
func TestValidateWarningMissingProvenance(t *testing.T) {
	fm := &Frontmatter{
		ID:        "REQ-004",
		Title:     "No sources",
		Type:      "spec",
		Status:    "draft",
		DCCreator: "bob",
	}
	issues := Validate(fm)
	warnings := filterSeverity(issues, SeverityWarning)
	found := false
	for _, w := range warnings {
		if w.Field == "provenance.sources" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning on field 'provenance.sources', got: %v", warnings)
	}
}

// TestValidateLegacySourceType checks that old source type gets a warning, not an error.
func TestValidateLegacySourceType(t *testing.T) {
	fm := &Frontmatter{
		ID:     "SRC-001",
		Title:  "A source page",
		Type:   "source",
		Status: "draft",
	}
	issues := Validate(fm)
	errs := filterSeverity(issues, SeverityError)
	for _, e := range errs {
		if e.Field == "type" {
			t.Errorf("legacy type 'source' should not produce error, got: %v", e)
		}
	}
	warnings := filterSeverity(issues, SeverityWarning)
	found := false
	for _, w := range warnings {
		if w.Field == "type" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected deprecation warning on legacy type 'source'")
	}
}

// TestValidateTrustLevelOutOfRange checks that trust_level outside 0-3 is an error.
func TestValidateTrustLevelOutOfRange(t *testing.T) {
	fm := &Frontmatter{
		ID:         "REQ-005",
		Title:      "Bad trust",
		Type:       "spec",
		Status:     "draft",
		TrustLevel: 5,
	}
	issues := Validate(fm)
	errs := filterSeverity(issues, SeverityError)
	found := false
	for _, e := range errs {
		if e.Field == "trust_level" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error on field 'trust_level', got: %v", errs)
	}
}

// TestValidateWarnMissingDCCreator checks that missing dc.creator produces a warning.
func TestValidateWarnMissingDCCreator(t *testing.T) {
	fm := &Frontmatter{
		ID:    "REQ-006",
		Title: "No creator",
		Type:  "spec",
		Status: "draft",
		Provenance: Provenance{
			Sources: []Source{{Ref: "prd-v1"}},
		},
	}
	issues := Validate(fm)
	warnings := filterSeverity(issues, SeverityWarning)
	found := false
	for _, w := range warnings {
		if w.Field == "dc.creator" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning on field 'dc.creator', got: %v", warnings)
	}
}

// TestHasErrors verifies HasErrors returns true only when error-severity issues are present.
func TestHasErrors(t *testing.T) {
	none := []Issue{}
	if HasErrors(none) {
		t.Error("HasErrors should be false for empty slice")
	}

	warnOnly := []Issue{{Severity: SeverityWarning, Field: "x", Message: "warn"}}
	if HasErrors(warnOnly) {
		t.Error("HasErrors should be false for warnings only")
	}

	withErr := []Issue{
		{Severity: SeverityWarning, Field: "x", Message: "warn"},
		{Severity: SeverityError, Field: "y", Message: "err"},
	}
	if !HasErrors(withErr) {
		t.Error("HasErrors should be true when an error is present")
	}
}

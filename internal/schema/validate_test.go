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

// TestValidateRequirement checks that a valid minimal requirement produces no errors.
func TestValidateRequirement(t *testing.T) {
	fm := &Frontmatter{
		ID:     "REQ-001",
		Title:  "My requirement",
		Type:   "requirement",
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

// TestValidateMissingRequired checks that missing id and type produce at least 2 errors.
func TestValidateMissingRequired(t *testing.T) {
	fm := &Frontmatter{
		Title:  "No ID or Type",
		Status: "draft",
	}
	issues := Validate(fm)
	errs := filterSeverity(issues, SeverityError)
	if len(errs) < 2 {
		t.Errorf("expected at least 2 errors, got %d: %v", len(errs), errs)
	}
	// Verify id and type fields are represented
	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	if !fields["id"] {
		t.Error("expected error on field 'id'")
	}
	if !fields["type"] {
		t.Error("expected error on field 'type'")
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
		Type:   "requirement",
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
		Type:      "requirement",
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

// TestValidateSourceTypeMissingSourceMeta checks type=source requires source_meta.
func TestValidateSourceTypeMissingSourceMeta(t *testing.T) {
	fm := &Frontmatter{
		ID:     "SRC-001",
		Title:  "A source without meta",
		Type:   "source",
		Status: "active",
	}
	issues := Validate(fm)
	errs := filterSeverity(issues, SeverityError)
	found := false
	for _, e := range errs {
		if e.Field == "source_meta" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error on field 'source_meta', got: %v", errs)
	}
}

// TestValidateTrustLevelOutOfRange checks that trust_level outside 0-3 is an error.
func TestValidateTrustLevelOutOfRange(t *testing.T) {
	fm := &Frontmatter{
		ID:         "REQ-005",
		Title:      "Bad trust",
		Type:       "requirement",
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
		Type:  "requirement",
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

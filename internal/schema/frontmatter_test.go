package schema

import (
	"testing"
	"time"
)

const fullFrontmatterYAML = `---
id: "test-001"
title: "Test Page"
type: "concept"
status: "draft"
dc.creator: "alice"
dc.created: "2024-01-15"
trust_level: 2
tags:
  - go
  - wiki
provenance:
  sources:
    - ref: "src-001"
      title: "Some Source"
      version: 1
      checksum: "abc123"
      retrieved: "2024-01-10"
      status: "active"
  contributors:
    - identity: "alice"
      role: "author"
      decision: "approved"
      date: "2024-01-15"
supersedes: "old-001"
updates:
  - "ref-001"
  - "ref-002"
---
This is the body content.
Second line of body.
`

func TestParseFrontmatter(t *testing.T) {
	fm, body, err := Parse([]byte(fullFrontmatterYAML))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	// Required fields
	if fm.ID != "test-001" {
		t.Errorf("ID: got %q, want %q", fm.ID, "test-001")
	}
	if fm.Title != "Test Page" {
		t.Errorf("Title: got %q, want %q", fm.Title, "Test Page")
	}
	if fm.Type != "concept" {
		t.Errorf("Type: got %q, want %q", fm.Type, "concept")
	}
	if fm.Status != "draft" {
		t.Errorf("Status: got %q, want %q", fm.Status, "draft")
	}

	// dc fields
	if fm.DCCreator != "alice" {
		t.Errorf("DCCreator: got %q, want %q", fm.DCCreator, "alice")
	}
	wantCreated := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	if !fm.DCCreated.Time.Equal(wantCreated) {
		t.Errorf("DCCreated: got %v, want %v", fm.DCCreated.Time, wantCreated)
	}

	// trust_level
	if fm.TrustLevel != 2 {
		t.Errorf("TrustLevel: got %d, want %d", fm.TrustLevel, 2)
	}

	// tags
	if len(fm.Tags) != 2 {
		t.Fatalf("Tags: got %d items, want 2", len(fm.Tags))
	}
	if fm.Tags[0] != "go" || fm.Tags[1] != "wiki" {
		t.Errorf("Tags: got %v, want [go wiki]", fm.Tags)
	}

	// provenance sources
	if len(fm.Provenance.Sources) != 1 {
		t.Fatalf("Provenance.Sources: got %d, want 1", len(fm.Provenance.Sources))
	}
	src := fm.Provenance.Sources[0]
	if src.Ref != "src-001" {
		t.Errorf("Source.Ref: got %q, want %q", src.Ref, "src-001")
	}
	if src.Title != "Some Source" {
		t.Errorf("Source.Title: got %q, want %q", src.Title, "Some Source")
	}
	if src.Version != 1 {
		t.Errorf("Source.Version: got %d, want 1", src.Version)
	}
	if src.Checksum != "abc123" {
		t.Errorf("Source.Checksum: got %q, want %q", src.Checksum, "abc123")
	}
	wantRetrieved := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	if !src.Retrieved.Time.Equal(wantRetrieved) {
		t.Errorf("Source.Retrieved: got %v, want %v", src.Retrieved.Time, wantRetrieved)
	}
	if src.Status != "active" {
		t.Errorf("Source.Status: got %q, want %q", src.Status, "active")
	}

	// provenance contributors
	if len(fm.Provenance.Contributors) != 1 {
		t.Fatalf("Provenance.Contributors: got %d, want 1", len(fm.Provenance.Contributors))
	}
	contrib := fm.Provenance.Contributors[0]
	if contrib.Identity != "alice" {
		t.Errorf("Contributor.Identity: got %q, want %q", contrib.Identity, "alice")
	}
	if contrib.Role != "author" {
		t.Errorf("Contributor.Role: got %q, want %q", contrib.Role, "author")
	}
	if contrib.Decision != "approved" {
		t.Errorf("Contributor.Decision: got %q, want %q", contrib.Decision, "approved")
	}
	wantContribDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	if !contrib.Date.Time.Equal(wantContribDate) {
		t.Errorf("Contributor.Date: got %v, want %v", contrib.Date.Time, wantContribDate)
	}

	// supersedes / updates
	if fm.Supersedes != "old-001" {
		t.Errorf("Supersedes: got %q, want %q", fm.Supersedes, "old-001")
	}
	if len(fm.Updates) != 2 {
		t.Fatalf("Updates: got %d, want 2", len(fm.Updates))
	}
	if fm.Updates[0] != "ref-001" || fm.Updates[1] != "ref-002" {
		t.Errorf("Updates: got %v", fm.Updates)
	}

	// body
	wantBody := "This is the body content.\nSecond line of body.\n"
	if string(body) != wantBody {
		t.Errorf("body: got %q, want %q", string(body), wantBody)
	}
}

func TestSerializeFrontmatter(t *testing.T) {
	fm := &Frontmatter{
		ID:        "roundtrip-001",
		Title:     "Round Trip Test",
		Type:      "policy",
		Status:    "active",
		DCCreator: "bob",
		DCCreated: Date{Time: time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC)},
		Tags:      []string{"alpha", "beta"},
		Provenance: Provenance{
			Sources: []Source{
				{
					Ref:    "src-rt",
					Title:  "RT Source",
					Status: "canonical",
				},
			},
		},
	}
	body := []byte("Round-trip body text.\n")

	data, err := Serialize(fm, body)
	if err != nil {
		t.Fatalf("Serialize error: %v", err)
	}

	// Parse back
	fm2, body2, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse after Serialize error: %v", err)
	}

	if fm2.ID != fm.ID {
		t.Errorf("ID round-trip: got %q, want %q", fm2.ID, fm.ID)
	}
	if fm2.Title != fm.Title {
		t.Errorf("Title round-trip: got %q, want %q", fm2.Title, fm.Title)
	}
	if fm2.Type != fm.Type {
		t.Errorf("Type round-trip: got %q, want %q", fm2.Type, fm.Type)
	}
	if fm2.Status != fm.Status {
		t.Errorf("Status round-trip: got %q, want %q", fm2.Status, fm.Status)
	}
	if fm2.DCCreator != fm.DCCreator {
		t.Errorf("DCCreator round-trip: got %q, want %q", fm2.DCCreator, fm.DCCreator)
	}
	if !fm2.DCCreated.Time.Equal(fm.DCCreated.Time) {
		t.Errorf("DCCreated round-trip: got %v, want %v", fm2.DCCreated.Time, fm.DCCreated.Time)
	}
	if len(fm2.Tags) != 2 || fm2.Tags[0] != "alpha" || fm2.Tags[1] != "beta" {
		t.Errorf("Tags round-trip: got %v", fm2.Tags)
	}
	if len(fm2.Provenance.Sources) != 1 || fm2.Provenance.Sources[0].Ref != "src-rt" {
		t.Errorf("Provenance.Sources round-trip: got %v", fm2.Provenance.Sources)
	}
	if string(body2) != string(body) {
		t.Errorf("body round-trip: got %q, want %q", string(body2), string(body))
	}
}

func TestParseNoFrontmatter(t *testing.T) {
	input := []byte("Just plain markdown.\nNo frontmatter here.\n")

	fm, body, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if fm != nil {
		t.Errorf("expected nil frontmatter, got %+v", fm)
	}
	if string(body) != string(input) {
		t.Errorf("body: got %q, want %q", string(body), string(input))
	}
}

func TestParseEmptyBody(t *testing.T) {
	input := []byte("---\nid: \"empty-body\"\ntitle: \"No Body\"\ntype: \"note\"\nstatus: \"draft\"\n---\n")

	fm, body, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if fm == nil {
		t.Fatal("expected non-nil frontmatter")
	}
	if fm.ID != "empty-body" {
		t.Errorf("ID: got %q, want %q", fm.ID, "empty-body")
	}
	if len(body) != 0 {
		t.Errorf("body: expected empty, got %q", string(body))
	}
}

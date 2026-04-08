# Phase 5: Steward Agents — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build three steward agent types — Lint, Resolution, and Ingest — that proactively maintain wiki quality. Stewards are standard wiki users (not special in the permission model) with tooling and instructions that make them proactive. Their behavioral rules are derived from the PHAT-TOAD framework.

**Architecture:** Steward agents are Go packages that use the wiki's MCP client or REST API. They can be run as CLI commands (`prd2wiki steward lint`), as scheduled jobs, or triggered by git hooks. They follow PHAT-TOAD's principle: standard agents with wider scope and specific tooling, not exceptions to the system.

**Tech Stack:** Go, existing wiki API client, PHAT-TOAD behavioral rules embedded as configuration

**Spec Reference:** `/srv/prd2wiki/docs/superpowers/specs/2026-04-07-prd2wiki-design-04.md` (Section 12)

**Prior Art:**
- `/srv/PHAT-TOAD-with-Trails/steward/system.md` — agent behavioral rules, anti-patterns
- `/srv/PHAT-TOAD-with-Trails/steward/steward-council.md` — mediation protocol
- `/srv/PHAT-TOAD-with-Trails/inspector/CLAUDE.md` — inspector pattern

---

## File Structure

```
internal/
└── steward/
    ├── steward.go             # Shared steward types, report format, behavioral rules
    ├── steward_test.go
    ├── lint.go                # Lint steward — provenance chain validation, contradiction detection
    ├── lint_test.go
    ├── resolve.go             # Resolution steward — contested page resolution
    ├── resolve_test.go
    ├── ingest.go              # Ingest steward — process incoming source material
    ├── ingest_test.go
    └── rules.go               # PHAT-TOAD behavioral rules as structured config
cmd/
└── prd2wiki/
    └── main.go                # Add steward subcommands
```

---

### Task 1: Steward Types + Behavioral Rules

**Files:**
- Create: `internal/steward/steward.go`
- Create: `internal/steward/rules.go`
- Create: `internal/steward/steward_test.go`

Shared types for all steward operations:

```go
package steward

type Finding struct {
    Severity  string `json:"severity"`   // error | warning | info
    PageID    string `json:"page_id"`
    Field     string `json:"field,omitempty"`
    Message   string `json:"message"`
    Action    string `json:"action,omitempty"` // suggested action
}

type Report struct {
    Steward   string    `json:"steward"`
    Project   string    `json:"project"`
    Timestamp time.Time `json:"timestamp"`
    Findings  []Finding `json:"findings"`
    Summary   string    `json:"summary"`
}
```

**rules.go** — PHAT-TOAD behavioral rules encoded as Go constants/documentation. These are embedded into steward prompts when stewards are used as agent system prompts:

```go
package steward

// Rules derived from PHAT-TOAD framework.
// These are injected into steward agent prompts when used as MCP-driven agents.
const (
    RuleVerifyBeforeClaiming = "Never claim to understand what you haven't verified. If you can describe but haven't operated, your understanding is incomplete."
    RuleConstraintsFirst     = "Before proposing HOW, ask what CANNOT change. Hard constraints shape every decision."
    RuleNoConcernsIsRedFlag  = "'No concerns' after reviewing complex content is a red flag. Walk through specifically how each claim survives your proposed change."
    RuleProvenanceTags       = "Tag inherited knowledge as [UNVERIFIED] if not personally verified against current state."
    RuleNoMutateRule         = "Delivered documents are immutable. Corrections go in new versions, not edits to delivered ones."
    RuleMechanicalGates      = "Gates must be mechanical, not advisory. Self-awareness of a tendency does not prevent it."
    RuleCleanVsComplete      = "Clean means it doesn't break. Complete means the next agent can continue without rediscovery."
)

// AntiPatterns from PHAT-TOAD system.md section 6.
var AntiPatterns = []string{
    "6.1 Confident Architect: Writing definitive descriptions from surface-level familiarity.",
    "6.2 Premature Builder: Offering to start before foundational questions are resolved.",
    "6.3 Shallow Agreement: Saying 'looks good' without walking through fragile components.",
    "6.5 Performative Compliance: Restating known information in a new format.",
    "6.7 Premature GO: Declaring readiness before all parties confirm all open items.",
    "6.8 Clean vs Complete: Treating documentation as optional after tests pass.",
}
```

Test: verify Report can be JSON marshaled, Finding severities are valid.

- [ ] Implement and commit

### Task 2: Lint Steward

**Files:**
- Create: `internal/steward/lint.go`
- Create: `internal/steward/lint_test.go`

The lint steward checks wiki health:

```go
type LintSteward struct {
    client *mcp.WikiClient
}

func NewLintSteward(client *mcp.WikiClient) *LintSteward

func (l *LintSteward) Run(project string) (*Report, error)
```

**Checks performed:**

1. **Provenance chain validation** — for each page, check that all cited sources exist and are not stale/deprecated
2. **Orphaned pages** — pages with no inbound references (not cited by anything)
3. **Deprecated source dependents** — pages citing deprecated/stale sources
4. **Missing required fields** — pages with conformance=pending
5. **Contradictions** — pages with status=contested that are unresolved

**Implementation:**
1. List all pages via `client.ListPages(project, nil)`
2. For each page, get references via `client.GetReferences(project, id, 1)`
3. Check each reference status
4. Build findings list
5. Generate summary report

**Test with mock API server:**
- Seed mock with pages that have broken provenance, orphans, etc.
- Run lint, verify correct findings

- [ ] Implement and commit

### Task 3: Resolution Steward

**Files:**
- Create: `internal/steward/resolve.go`
- Create: `internal/steward/resolve_test.go`

The resolution steward handles contested/stale pages:

```go
type ResolveSteward struct {
    client *mcp.WikiClient
}

func NewResolveSteward(client *mcp.WikiClient) *ResolveSteward

func (r *ResolveSteward) Run(project string) (*Report, error)
```

**What it does:**

1. Find all pages with status=contested or status=stale
2. For each contested page: report what's contested and what evidence exists
3. For each stale page: identify which sources changed and suggest next steps
4. Generate findings with recommended actions:
   - "Re-verify source X and update checksum if still valid"
   - "Source X has been deprecated — update page to remove dependency"
   - "Challenge branch exists at challenge/topic — needs review"

The resolution steward does NOT auto-fix — it produces a report of what needs attention. (Full automated resolution with LLM reasoning is future work.)

- [ ] Implement and commit

### Task 4: Ingest Steward

**Files:**
- Create: `internal/steward/ingest.go`
- Create: `internal/steward/ingest_test.go`

The ingest steward processes pages on `ingest/*` branches:

```go
type IngestSteward struct {
    client *mcp.WikiClient
}

func NewIngestSteward(client *mcp.WikiClient) *IngestSteward

func (i *IngestSteward) Run(project string) (*Report, error)
```

**What it does:**

1. List pages on ingest/* branches (via client.ListPages with branch filter)
2. For each ingested page:
   - Check schema conformance
   - Check for duplicates (via wiki_search for similar titles)
   - Verify source metadata is complete (for type=source pages)
3. Generate findings:
   - "Ingested page X is schema-conformant, ready for draft promotion"
   - "Ingested page X is missing source_meta.url"
   - "Ingested page X may duplicate existing page Y"

- [ ] Implement and commit

### Task 5: Steward CLI Subcommands

**Files:**
- Modify: `cmd/prd2wiki/main.go` — add steward subcommands
- Modify: `Makefile` — no changes needed (same binary)

Add steward subcommands to the main binary:

```bash
prd2wiki steward lint --project default        # run lint steward
prd2wiki steward resolve --project default     # run resolution steward
prd2wiki steward ingest --project default      # run ingest steward
prd2wiki steward all --project default         # run all stewards
```

Implementation: check `os.Args` for "steward" subcommand before starting the server. If present, run the steward and exit instead of starting the HTTP server.

```go
func main() {
    if len(os.Args) > 1 && os.Args[1] == "steward" {
        runSteward(os.Args[2:])
        return
    }
    // ... existing server startup ...
}

func runSteward(args []string) {
    // Parse steward subcommand and flags
    // Create WikiClient pointing to running wiki server
    // Run appropriate steward
    // Print report as JSON
}
```

- [ ] Implement and commit

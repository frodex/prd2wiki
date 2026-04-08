# prd2wiki — Design Specification

**Date:** 2026-04-07
**Status:** Draft
**Version:** 0.3
**Preceding:** 0.2 (2026-04-07-prd2wiki-design-02.md)
**Changes from 0.2:** Added tree-based reference display for both hard and soft references (Section 14.3). References are navigable graphs with expandable depth, not flat lists. API supports `depth` parameter for recursive traversal. Stale/contested flags propagate visually through the tree.

---

## 1. Purpose

prd2wiki is core infrastructure for agentic development. It provides a persistent, curated, multi-project knowledge backbone where AI agents and humans collaboratively maintain living documents — PRDs, requirements, research, decisions — with full provenance, history, and reasoning.

The system serves as a **source of truth** that agents can read, write, challenge, and reason about. It encourages dialog: agents are expected to challenge the source of truth when they find contradictions, errors, or new information. The truth branch is highly curated; input is open.

**prd2wiki is not a text editor on a filesystem.** It is a persistent context and intent preserving resource with deep insight into user intent that can be mined across projects for insight. The dual input stream — agentic and human — operating over the same content makes it a unique tool for planning, research, and knowledge curation that expands beyond the chat interface.

### 1.1 What This Is

- A **work surface for planning and research** — captures the birth process of a PRD, not just the final document. Brainstorming loops, rejected approaches, reasoning, and anti-patterns are preserved as first-class records.
- A git-backed wiki with What You See Is What You Get (WYSIWYG) web User Interface (UI) for humans and Model Context Protocol (MCP) interface for agents
- A knowledge management system with provenance-based trust, source pinning, and self-healing
- A multi-project, multi-team platform with standard Role-Based Access Control (RBAC)
- A librarian-managed data pipeline that classifies, validates, normalizes, and indexes all content

### 1.2 What This Is Not

- Not a static document generator
- Not a chat-based knowledge system (no conversation storage)
- Not a vector database with a UI (vectors are a derived index, not the content store)
- Not a text editor on a filesystem

### 1.3 Design Principles

1. **Familiar, trusted building blocks.** Use established standards (git, markdown, RBAC, MCP, Dublin Core, MADR). Only invent where nothing exists. People should recognize this as a natural evolution of things they already know.

2. **Composable, recursive architecture.** New features stem from a robust set of building blocks adapted via configuration, not rewrites. Everything that can be a wiki page is a wiki page — sources, decisions, vocabulary, trust policies. One implementation, adapters absorb differences (PHAT-TOAD Rule 1).

3. **Git is truth.** All content lives in git as markdown files. Everything else — SQLite index, vector embeddings, computed metadata — is derived, disposable, and rebuildable from git.

4. **Schema is the type system.** Draft branches are dynamically typed (flexible, fast). Truth branch is statically typed (strict, reliable). Nothing merges to truth without full schema compliance.

5. **Provenance is first-class.** Every page records what it was built from, with checksums pinning exact versions. When sources mutate, the system detects it automatically and cascades invalidation.

6. **The librarian never silently changes content.** Every mutation is previewed. Users and agents are always in control.

7. **Dual-input is the primary interface.** Human and agent contributions flow through the same system, are subject to the same rules, and are visible to each other. The wiki is the shared work surface, not a destination for finished artifacts.

---

## 2. Work Surface for Planning & Research

### 2.1 The Problem

Valuable information is wasted in the agentic planning and rework process. Brainstorming loops reveal intent, reasoning, and anti-patterns rejected along the way — but this knowledge dies in chat transcripts. When context windows fill and sessions end, the birth process of a PRD is lost. Only the final artifact survives, stripped of the reasoning that produced it.

This is the same problem PHAT-TOAD identified: "Modern AI tooling treats context as a session-scoped blob." prd2wiki solves it by making the wiki the work surface itself, not just the destination for finished documents.

### 2.2 How It Works

The wiki is an interactive tool during brainstorming, planning, and research — not just a place where polished results are stored afterward. Both humans and agents contribute to the same pages in real-time through their respective interfaces.

**During a brainstorming/planning session:**

```
Human (browser)                    Agent (MCP)
     │                                  │
     │  Opens wiki page in WYSIWYG      │  Reads page via wiki_read
     │  Adds direction: "consider       │
     │  WebSockets instead of polling"  │
     │                                  │  Sees the update
     │                                  │  Researches, reasons
     │                                  │  Updates draft page via
     │                                  │  wiki_propose (integrate)
     │  Sees agent's analysis           │
     │  Adds feedback, redirects        │
     │                                  │  Adjusts, proposes again
     │                                  │
     └──────────┬───────────────────────┘
                │
         Draft pages accumulate:
         - research findings
         - rejected approaches (with WHY)
         - decisions (MADR format)
         - evolving requirements
         - anti-patterns discovered
                │
         When ready: CR → truth
         Birth process preserved in git history
```

### 2.3 What Gets Preserved

Unlike chat transcripts that are compressed and discarded:

- **Research findings** — what was investigated, what was learned, sources consulted
- **Rejected approaches** — what was considered and why it was rejected (as `[!decision]` blocks)
- **Reasoning chains** — how conclusions were reached, what evidence supported them
- **Anti-patterns discovered** — what was tried and failed, with why
- **Intent evolution** — how requirements changed as understanding deepened
- **Human direction** — guidance, corrections, and redirections from the human collaborator
- **Agent analysis** — research, comparisons, and recommendations from agents

All of this is versioned in git, tagged with provenance, and searchable. A future agent or human can trace not just what was decided, but the full reasoning path that led there.

### 2.4 Cross-Project Insight Mining

Because the wiki spans multiple projects with consistent schema and provenance:

- Patterns emerge across projects (e.g., "every project that uses JWT encounters the same session timeout issue")
- Anti-patterns discovered in one project are discoverable by agents working on others
- Research done for Project A doesn't need to be repeated for Project B — it's a citable source
- Intent and reasoning from brainstorming sessions can be queried semantically via the vector index

### 2.5 Sidecar API for Agent-Attached Applications

The Core API must support sidecar applications that attach directly to agents during brainstorming sessions. A sidecar app could:

- Present a focused web UI scoped to the current brainstorming context
- Show the human what the agent is working on in real-time
- Accept structured input from the human (not just free-text chat)
- Feed that input directly to the agent via the wiki

This is a future application phase, but the API must be designed to support it from the start. The MCP sidecar is the first example of this pattern; agent-attached brainstorming UIs are the next.

---

## 3. System Architecture

### 3.1 Components

```
┌───────────────────────────────────────────────────┐
│                    prd2wiki                        │
│                                                    │
│  ┌───────────────────┐                            │
│  │  Git Bare Repos    │                            │
│  │  (markdown files   │                            │
│  │   = actual content)│                            │
│  └────────┬───────────┘                            │
│           │ read/write                             │
│           ▼                                        │
│  ┌────────────────────┐    ┌───────────────────┐  │
│  │    Librarian        │──▶│  Computed Indexes  │  │
│  │                     │   │                    │  │
│  │  classify           │   │  SQLite            │  │
│  │  validate           │   │   - metadata       │  │
│  │  normalize          │   │   - FTS5           │  │
│  │  deduplicate        │   │   - provenance     │  │
│  │  cross-reference    │   │   - graph edges    │  │
│  │  index              │   │                    │  │
│  └────────┬────────────┘   │  LanceDB           │  │
│           │                │   - embeddings      │  │
│    ┌──────┴──────┐        │   - page pointers   │  │
│    │  Core API   │        └───────────────────┘  │
│    │  (net/http) │                                │
│    └──────┬──────┘                                │
│      ┌────┼────────┐                              │
│    ┌─┴──┐ │    ┌───┴────┐                        │
│    │Web │ │    │Future  │                        │
│    │UI  │ │    │Sidecars│                        │
│    └────┘ │    └────────┘                        │
│       ┌───┴───┐                                  │
│       │ MCP   │                                  │
│       │Sidecar│                                  │
│       └───────┘                                  │
└───────────────────────────────────────────────────┘
```

**Wiki Core** — Go binary using standard library `net/http` + `golang.org/x/*` extensions. Owns git operations via `go-git`. Goldmark for markdown parsing. One git bare repo per project/namespace.

**Librarian** — Core component (not a sidecar). Sits between the API and storage. Every write passes through it. Implements: classification, schema validation, normalization, deduplication, cross-referencing, index management. Adapted from the Pippi Knowledge Librarian pattern.

**Computed Indexes** — Both are derived from git content, disposable, rebuildable:
- **SQLite**: page metadata, full-text search (FTS5), provenance graph edges, document relationships, page status
- **LanceDB**: embeddings (mathematical representations of content) + page pointers. Powers semantic search, similarity-based dedup, prior art suggestions. Does NOT store markdown content.

**Core API** — Representational State Transfer (REST) API using standard library `net/http`. Serves both the web UI and sidecars. Create, Read, Update, Delete (CRUD) for pages, search, provenance queries, branch operations, challenge management. **Designed to support sidecar applications** that attach to agents for brainstorming, planning, and other interactive workflows.

**Sidecars** — independently deployable processes that speak to the Core API:
- **Web UI**: Milkdown WYSIWYG editor (markdown-native, ProseMirror + remark, MIT). Served as static assets + API calls.
- **MCP Sidecar**: Exposes wiki as MCP resources/tools. Speaks MCP to agents, REST to wiki core. Can be updated/replaced without recompiling the core.
- **Future sidecars**: CLI, webhooks, Slack, agent-attached brainstorming UIs, etc. All attach the same way.

### 3.2 Technology Stack

| Component | Technology | Why |
|---|---|---|
| Language | Go + `golang.org/x/*` | User preference, no third-party web framework |
| Git operations | `go-git/go-git` | Pure Go git implementation |
| Markdown parsing | `yuin/goldmark` | MIT, extensible, used by Hugo & Gitea |
| Web framework | `net/http` (stdlib) | No dependencies |
| Structured index | SQLite | Embedded, no server process |
| Vector index | LanceDB | Embedded, hybrid search, MIT |
| WYSIWYG editor | Milkdown | Markdown-native, ProseMirror + remark, MIT |
| Binary file storage | Git LFS | Industry standard for binary files in git |
| Deployment | Single binary + Docker/Compose | Self-hosted |

---

## 4. Content Model

### 4.1 Page Format

Every wiki page is a markdown file (CommonMark + GitHub Flavored Markdown) with structured YAML frontmatter.

```yaml
---
id: PRD-042
title: "Authentication Requirements"
type: requirement          # concept | task | reference | decision | requirement | source | config
status: active             # draft | review | active | contested | stale | superseded | deprecated

# Dublin Core (ISO 15836)
dc.creator: "jane@example.com"
dc.created: 2026-03-15
dc.modified: 2026-04-01
dc.rights: "internal"

# Provenance (W3C PROV-DM vocabulary)
provenance:
  sources:
    - ref: "wiki://project-a/auth-research"
      version: 3
      checksum: "sha256:abc123..."
      retrieved: 2026-03-20
      status: valid
    - ref: "https://datatracker.ietf.org/doc/html/rfc6749"
      title: "OAuth 2.0 Framework"
      checksum: "sha256:789ghi..."
      retrieved: 2026-04-01
      status: valid
  contributors:
    - identity: "jane@example.com"
      role: author
    - identity: "agent-review-7"
      role: reviewer
      decision: approved
      date: 2026-03-28

# RFC-style lifecycle
supersedes: PRD-031
superseded_by: null
updates: [PRD-028, PRD-033]

# Knowledge Supply Chain Level (adapted from SLSA)
trust_level: 3             # 0=unattributed, 1=attributed+cited, 2=signed+verified, 3=reviewed+approved

# Schema conformance
conformance: valid         # valid | pending | failed

tags: [authentication, security, mvp]
---

# Authentication Requirements

[page content in CommonMark + GFM markdown]
```

### 4.2 Document Types

Adapted from Darwin Information Typing Architecture (DITA) topic typing. All types use the same storage, versioning, and provenance model — the `type` field drives rendering and validation rules.

| Type | Purpose |
|---|---|
| `requirement` | Product/system requirements |
| `concept` | Explanatory/background information |
| `task` | How-to procedures |
| `reference` | Lookup information (APIs, schemas, configs) |
| `decision` | Decision records (Markdown Any Decision Records format) — context, options, decision, consequences |
| `source` | Registered external sources (bibliography entries) |
| `config` | Wiki configuration (trust policies, vocabulary, schemas) |

### 4.3 Structured Blocks

Inline structured content uses GitHub's alert syntax extended with custom types:

```markdown
> [!decision] Use JWT for session tokens
> **Status:** accepted
> **Date:** 2026-03-20
> **Deciders:** jane@example.com, john@example.com
>
> Chose JWT over opaque tokens because the system needs
> stateless verification across services.
> **Rejected:** opaque tokens (require shared session store)

> [!challenge] Conflicts with session timeout in PRD-028
> **Raised by:** agent-lint-3
> **Date:** 2026-03-22
> **Status:** open
> **Evidence:** PRD-028 §3.2 requires server-side session invalidation,
> which contradicts stateless JWT verification.

> [!stale] Source changed: wiki://project-b/api-spec
> **Detected by:** steward-lint-3
> **Date:** 2026-04-05
> **Expected:** v7 (sha256:def456...)
> **Current:** v8 (sha256:xyz789...)
> **Impact:** UNKNOWN — needs steward review

> [!deprecated] Auto-deprecated by steward-lint-3
> **Date:** 2026-04-05
> **Reason:** 3 of 4 sources have been superseded.
> **Action required:** Owner must rebuild against current sources.
```

### 4.4 Trust Levels (adapted from Supply-chain Levels for Software Artifacts)

| Level | Meaning | Requirements |
|---|---|---|
| L0 | Unattributed | Content exists, no provenance metadata |
| L1 | Attributed + cited | Has author, sources listed |
| L2 | Signed + verified | Commit signed, sources independently checked |
| L3 | Reviewed + approved | Went through Change Request protocol, steward validated provenance chain |

Only L3 content can be merged to the truth branch.

---

## 5. Branching & Truth Model

### 5.1 Branch Structure

```
truth              ← published source of truth (L3 only, schema-conformant)
├── draft/*        ← work in progress (any trust level, flexible schema)
├── challenge/*    ← disputes with evidence
├── ingest/*       ← incoming data from external sources
└── steward/*      ← steward agent work branches
```

### 5.2 Content Flow

```
External data ──▶ ingest/source-name ──▶ draft/topic ──▶ truth
Agent research ──▶ draft/topic ──────────────────────────┘
Human editing  ──▶ draft/topic ──────────────────────────┘
                                              │
                                         CR Protocol
                                              │
                                    Schema valid?
                                    Provenance chain intact?
                                    Trust level = L3?
                                    No contradictions?
                                              │
                                         ▼ merge
                                         truth
```

### 5.3 Change Request (CR) Protocol

Modeled on pull requests, enforced mechanically:

1. Author creates a draft or challenge branch with changes
2. Each changed page must have complete frontmatter (provenance, sources, trust level)
3. Librarian validates: schema compliance, provenance chain intact, no broken references, sources not deprecated
4. Steward agent or human reviewer evaluates the CR
5. If provenance chain traces back to vetted sources without breaks → approval can be automated
6. If new unvetted sources or contradictions exist → requires review
7. Merge to truth only when all gates pass

### 5.4 Challenge Flow

When an agent or human finds contested data:

1. Create `challenge/topic-name` branch
2. On that branch, the contested page gets `status: contested` and a `[!challenge]` block with evidence
3. The challenge references specific sources and explains the contradiction
4. On the truth branch, the affected page gets a contest flag in frontmatter: `contested_by: challenge/topic-name`
5. Truth page content stays unchanged, but the flag is visible to all readers and agents
6. Steward agent picks up contested pages, solicits evidence, resolves
7. Resolution either: merges the challenge (truth updates) or closes it (challenge rejected with reasoning captured as a `[!decision]` block)

### 5.5 Cascading Invalidation

When a source is deprecated or challenged:

1. Index query: "what pages cite this source?"
2. All dependent pages get flagged for re-evaluation
3. Their trust levels may drop (L3 → L1 if a source is no longer verified)
4. Steward agents are notified to investigate
5. When the owning agent updates affected sections, trust restores and healing cascades downstream

---

## 6. Pages as Build Artifacts — Source Pinning and Self-Healing

### 6.1 The Principle

Every page is an **as-built artifact** with a manifest. Like a Docker image with a Dockerfile — you know exactly what went into it, and you can tell when the inputs have changed. The wiki doesn't just version content — it tracks the validity of content against its sources continuously.

### 6.2 Source Pinning

Each source in `provenance.sources` records:
- The reference (wiki page, URL, file path)
- The version/commit at time of use
- A checksum of the source content
- The retrieval date

### 6.3 Staleness Detection

| Source Type | How Pinned | How Staleness Detected |
|---|---|---|
| Wiki page (same project) | Page ID + version + checksum | Git hook on commit — immediate |
| Wiki page (cross-project) | Project + page ID + version + checksum | Periodic poll or webhook between projects |
| External URL | URL + checksum + retrieval date | Periodic fetch and re-checksum |
| Source code file | File path + commit hash + checksum | Git hook on commit to source repo |
| Local file (PDF, doc) | Path + checksum, local copy in `_sources/` | Local copy preserved; staleness if original changes |

### 6.4 Impact Assessment

When staleness is detected, the steward agent:

1. Diffs the old and new versions of the changed source
2. Determines impact:
   - **No impact:** Change doesn't affect the page's claims. Clear the flag, update checksum.
   - **Partial impact:** Some sections still valid, some not. Mark specific sections, explain what's accurate and what's not.
   - **Full invalidation:** Change fundamentally breaks the page's claims. Flag for owner to rebuild.

### 6.5 Self-Deprecation

If a page's sources are thoroughly invalidated, the system auto-sets `status: deprecated` with an explanation and required action.

### 6.6 Local Source Copies

External sources get a local copy when possible, stored in `_sources/` or `_attachments/`. Preserves what the page was built from even if the external source disappears.

---

## 7. Sources as Wiki Pages (Recursive Architecture)

Sources/bibliography entries are wiki pages with `type: source`:

```yaml
---
id: SRC-RFC6749
title: "OAuth 2.0 Authorization Framework"
type: source
status: active
trust_level: 3

source_meta:
  url: "https://datatracker.ietf.org/doc/html/rfc6749"
  kind: standard           # standard | paper | documentation | observation | agent-research | cross-project
  authority: ietf
  retrieved: 2026-03-15
  verified_by: "jane@example.com"

tags: [oauth, authentication, ietf]
---
```

This means:
- Sources get the same trust levels, provenance, challenge mechanisms, and lifecycle as any other page
- The bibliography is just a query: `type: source`
- Decision log = query: `type: decision`
- Challenge tracker = query: `status: contested`
- Agent roster = query: `type: identity` (if used)
- Wiki configuration = `type: config` pages

Adding a new "feature" (glossary, risk register, meeting notes) is: define a new `type` value, optionally add rendering rules. No code changes, no new tables, no new APIs.

---

## 8. Cross-Project Trust

Trust does not propagate across project boundaries by default. Project A's truth is just another source from Project B's perspective.

```yaml
---
id: SRC-PROJ-A-042
title: "Project A — Authentication Requirements"
type: source
status: active
trust_level: 1              # B's assessment, NOT inherited from A

source_meta:
  kind: cross-project
  origin_project: project-a
  origin_id: PRD-042
  origin_trust_level: 3     # what A thinks of it
  local_assessment:
    evaluated_by: "bob@example.com"
    accepted_scope: "OAuth flow only"
    rejected_scope: "JWT claims structure"
---
```

Trust agreements between projects are `type: config` pages — versioned, auditable, challengeable, approved through the same CR protocol.

---

## 9. Access Control

### 9.1 Model

Standard RBAC with hierarchical content scoping. Three layers evaluated top-down:

1. **System role** — what kind of user (admin, editor, viewer, guest)
2. **Project scope** — what can your group do in THIS project
3. **Page override** — does THIS page restrict further (optional allowlist)

### 9.2 Roles

| Role | Capabilities |
|---|---|
| `admin` | Manage users, config, structure, all project operations |
| `editor` | Read, write, propose, challenge, approve CRs |
| `viewer` | Read only |
| `guest` | Access to specifically shared pages only |

### 9.3 Project Scoping

```yaml
# config/projects.yaml
projects:
  project-a:
    admins: [jane@example.com]
    editors: [team-engineering, agent-lint-3@service.internal]
    viewers: ["*"]
```

### 9.4 Page-Level Overrides

Optional, restrict-only:

```yaml
# In page frontmatter
access:
  restrict_to: [team-security, jane@example.com]
```

### 9.5 Authentication

OpenID Connect (OIDC) / OAuth for both humans and agents. Identity provider manages users and groups (Keycloak, Okta, GitHub, etc.). The wiki does not manage user accounts or group membership. Same permission evaluation for API and UI. Tokens are user-scoped.

### 9.6 Quality Gates (Separate from Permissions)

Branch protection rules enforce content quality. A user might have `editor` role but their merge still fails if:

- Schema validation fails
- Provenance chain is broken
- Trust level < L3
- Required reviewers haven't signed off

Quality gates = merge rules, not access rules.

---

## 10. Librarian

### 10.1 Role

The librarian is a core component that sits between the Core API and storage. Every write passes through it. It implements the ingestion pipeline adapted from the Pippi Knowledge Librarian.

### 10.2 Pipeline

```
classify → validate → normalize → deduplicate → cross-reference → write → index
```

### 10.3 Submission Modes

Three modes give explicit control over what the system does with input:

| Mode | UI Label | Librarian Behavior |
|---|---|---|
| **Verbatim** | `Submit [Do Not Mutate]` | Schema validation (flag, don't block). Save with `conformance: pending`. Index. No normalization. |
| **Conform** | `Submit [Correct & Format]` | Validate + normalize spelling/format/tags + dedup check. Return diff for preview. No content changes. |
| **Integrate** | `Submit [Reason & Merge]` | Full pipeline — validate + normalize + cross-reference + reason about placement + suggest merges. Return diff for preview. |

**Mandatory diff preview** for conform and integrate modes. The user/agent sees exactly what changed before confirming. Can accept, reject, or switch to verbatim.

### 10.4 Progressive Classification (adapted from Pippi)

1. **Rule-based** (< 1ms): frontmatter `type` field, structural pattern matching
2. **Vector similarity** (< 10ms): embed and compare against existing pages in LanceDB
3. **LLM escalation** (budgeted): only for `integrate` mode when confidence < 0.7

### 10.5 Vocabulary Management

Canonical terms for tags, relationship predicates, and document types stored in a vocabulary table (SQLite). Fuzzy matching normalizes incoming terms (> 0.85 similarity = normalize to canonical). New terms logged for review.

### 10.6 Deduplication

Keep-both-linked strategy: near-duplicates with conflicting data are linked as potential duplicates, never auto-merged. Steward agents or humans resolve. Merge execution is explicit with per-field resolution.

### 10.7 Schema Enforcement by Branch

| Branch | Schema Enforcement |
|---|---|
| `truth` | Strict — full compliance required, merge blocked if validation fails |
| `draft/*` | Flexible — non-conforming pages saved with `conformance: pending` |
| `challenge/*` | Partial — challenge metadata must conform, challenged content may be in flux |
| `ingest/*` | Flexible input, strict output — ingest steward produces conforming pages |

### 10.8 Fast-Track Bypass

System-internal writes (steward lint results, computed index updates, automated staleness flags) bypass the full pipeline. Only token verification + schema matching.

---

## 11. MCP Sidecar

### 11.1 Resources (read-only context)

- `wiki://project-a/PRD-042` — full page content + frontmatter
- `wiki://project-a/index` — page catalog for a project
- `wiki://project-a/graph` — provenance/dependency graph
- `wiki://project-a/contested` — pages with active challenges

### 11.2 Tools (agent-invocable actions)

- `wiki_search` — full-text + semantic search across projects
- `wiki_read` — read a specific page
- `wiki_propose` — create/update a page on a draft branch (accepts `intent: verbatim|conform|integrate`)
- `wiki_challenge` — raise a challenge against a page with evidence
- `wiki_ingest` — submit external source material for processing
- `wiki_lint` — run provenance/consistency checks
- `wiki_status` — check CR status, challenge status
- `wiki_log` — append to the operation log

### 11.3 Prompts (reusable workflows)

- `review_page` — structured review template
- `ingest_source` — guided source registration workflow

The sidecar is a separate process. Can be written in Go or TypeScript (whatever best supports the MCP SDK). Updated independently from the core.

---

## 12. Steward Agents

Steward agents are standard wiki users with `editor` role. They're not special in the permission model — they just have tooling and instructions that make them proactive.

### 12.1 Lint Steward

Runs periodically or on-commit:
- Validates provenance chains (are all cited sources still active?)
- Detects contradictions between pages
- Flags orphaned pages (no inbound references)
- Checks for deprecated sources with live dependents
- Runs staleness detection on external URLs
- Reports via `wiki_log`, creates challenges when issues found

### 12.2 Resolution Steward

Activated when contested pages exist:
- Solicits evidence from agents and humans
- Evaluates contradicting claims against sources
- Proposes resolution as a draft, creates `[!decision]` blocks with reasoning
- Does NOT auto-merge — submits CR for review

### 12.3 Ingest Steward

Processes incoming data:
- Reads submitted source material
- Creates source pages (registers in bibliography)
- Extracts key information, creates/updates wiki pages
- Cross-references with existing content
- Follows Karpathy pattern: update 10-15 related pages per ingest

All stewards follow PHAT-TOAD's principle: standard agents with wider scope and specific tooling, not exceptions to the system.

---

## 13. Media & Attachments

### 13.1 Storage

Git Large File Storage (LFS) for binary files. Media is versioned alongside content in the same commit history.

### 13.2 Directory Structure

```
pages/
  product/
    roadmap.md
    roadmap/
      _attachments/
        architecture-diagram.png
        q3-plan.pdf
      _sources/
        rfc6749-local-copy.pdf
  _shared/
    images/
      company-logo.png
```

### 13.3 References

Standard relative markdown paths: `![Diagram](roadmap/_attachments/diagram.png)`. No UUIDs, no custom syntax.

### 13.4 File Type Whitelist (enforced by git hook)

- Images: png, jpg, gif, webp, svg
- Documents: pdf
- Audio/Video: mp3, mp4, webm, ogg
- Data: csv, json, yaml, toml
- Code: py, js, ts, go, rs, sh, sql, etc.
- Reject: exe, dll, .DS_Store, node_modules

### 13.5 Size Limits (enforced by git hook)

- Warn > 10 MB
- Reject > 50 MB
- Large video/audio: link externally

---

## 14. Hard References & Soft References

### 14.1 Hard References (Provenance)

Explicit, intentional citations declared by the author in frontmatter `provenance.sources`. These ARE the trust chain:

- Checksummed and version-pinned
- Staleness detection and cascading invalidation apply
- Stored in git (frontmatter YAML)
- Author deliberately cited this source: "I built this page using these"

### 14.2 Soft References (Discovery)

System-generated suggestions surfaced by the vector DB based on semantic similarity. These are NOT part of the provenance chain:

- Computed from embeddings — "pages related to this topic"
- No checksums, no staleness tracking
- Discovery aid for readers and agents, not a trust claim
- Recalculated periodically as new content enters the wiki

### 14.3 Display — Reference Trees

References are not flat lists — they are navigable trees. A page cites sources, those sources cite their own sources, and so on. The tree is an expandable view of the provenance graph, rooted at the current page.

**Default view:** collapsed (direct references only). Click to expand and see deeper levels.

```
Hard References (provenance):
  ▶ SRC-RFC6749 "OAuth 2.0 Framework" (v1) ✓
  ▼ PRD-028 "Session Management" (v3) ✓
    ├── SRC-RFC6265 "HTTP Cookies" (v1) ✓
    ├── PRD-015 "User Identity Model" (v5) ⚠ STALE
    │   ├── SRC-OIDC-SPEC "OpenID Connect Core" ✓
    │   └── decision/identity-provider-choice ✓
    └── research/session-persistence-analysis (v2) ✓
  ▶ research/auth-analysis (v2) ✓

Soft References (discovered):
  ▶ PRD-099 "Session Management" (0.89)  ☑
  ▶ project-b/auth-design (0.84)         ☐ "Different paradigm"
  ▶ decision/jwt-vs-opaque (0.78)        ☑
```

**Tree behaviors:**

- **▶** collapsed — shows the top-level reference with status indicator
- **▼** expanded — shows that reference's own references, recursively
- Each level shows version, checksum status, and trust level
- **Stale/contested flags propagate visually** — if `PRD-015` deep in the tree is stale, the ⚠ indicator is visible even before expanding, so you can see at a glance if any dependency in the chain has problems
- Soft references also expand into trees — expanding a soft ref shows ITS hard references, revealing WHY the system thinks it's related

**API support:**

```
GET /api/pages/PRD-042/references?depth=3
```

```json
{
  "hard": [
    {
      "ref": "PRD-028",
      "title": "Session Management",
      "version": 3,
      "checksum": "sha256:def456...",
      "status": "valid",
      "trust_level": 3,
      "children": [
        {
          "ref": "SRC-RFC6265",
          "title": "HTTP Cookies",
          "version": 1,
          "status": "valid",
          "children": []
        },
        {
          "ref": "PRD-015",
          "title": "User Identity Model",
          "version": 5,
          "status": "stale",
          "stale_since": "2026-04-05",
          "children": [
            {"ref": "SRC-OIDC-SPEC", "status": "valid", "children": []},
            {"ref": "decision/identity-provider-choice", "status": "valid", "children": []}
          ]
        }
      ]
    }
  ],
  "soft": [
    {
      "ref": "PRD-099",
      "title": "Session Management",
      "similarity": 0.89,
      "accepted": true,
      "children": []
    }
  ]
}
```

The `depth` parameter controls tree depth. Default `1` (direct references only). The UI fetches deeper levels on expand — lazy loading, not upfront. The data comes from the SQLite provenance graph (hard refs) and vector DB (soft refs).

### 14.4 Soft Reference Dismissal

Users and agents can mark soft references as not valid — simple checkbox with optional reason field:

- **Checked (☑)** = accepted, valid/useful. Pinned — continues appearing even if similarity score drifts.
- **Unchecked (☐)** = dismissed. Stops appearing for this page. Does not resurface unless the dismissed page is substantially rewritten.
- **Reason field** = optional text explaining why the reference was dismissed. Helps steward agents understand patterns in dismissals.

Dismissals are stored in a sidecar file alongside the page (e.g., `PRD-042.soft-refs.yaml`), git-tracked for auditability:

```yaml
# PRD-042.soft-refs.yaml
dismissed:
  - ref: "project-b/auth-design"
    dismissed_by: "jane@example.com"
    date: 2026-04-05
    reason: "Different auth paradigm, not applicable"
accepted:
  - ref: "PRD-099"
    accepted_by: "jane@example.com"
    date: 2026-04-05
  - ref: "decision/jwt-vs-opaque"
    accepted_by: "agent-review-7"
    date: 2026-04-06
```

### 14.5 Soft-to-Hard Promotion

An accepted soft reference can be **promoted to a hard reference** with a single action. The system:

1. Reads the current version and computes a checksum of the referenced page
2. Adds a full provenance entry to the page's frontmatter
3. Removes the soft reference entry (it's now a hard reference)
4. Staleness tracking begins immediately

This creates a discovery → validation → trust pipeline:

```
Vector DB surfaces soft ref → user reviews → accept/dismiss → optionally promote to hard ref
```

Soft references that are frequently accepted across many pages may indicate a missing hard reference pattern — the lint steward can detect and suggest this.

---

## 15. Standards Adopted

| Standard | How Used | Maturity |
|---|---|---|
| **MCP** (Model Context Protocol, spec 2025-11-25) | Agent interface — resources, tools, prompts | Production, 97M monthly SDK downloads |
| **CommonMark + GFM** (GitHub Flavored Markdown) | Document format | Mature, universal |
| **YAML frontmatter** | Document metadata | De facto standard |
| **Dublin Core** (ISO 15836) | Metadata vocabulary (title, creator, date, source) | ISO standard |
| **Conventional Commits** | Structured commit messages | Widely adopted |
| **MADR v4** (Markdown Any Decision Records) | Decision record format | MIT + CC0, v4.0.0 |
| **W3C PROV-DM** (Provenance Data Model) | Provenance vocabulary (entity, activity, agent, wasDerivedFrom) | W3C Recommendation |
| **Gitsign/Sigstore** | Identity-based commit signing, keyless via OIDC | Production |
| **Git LFS** (Large File Storage) | Binary file storage in git | Industry standard |
| **OIDC** (OpenID Connect) / OAuth | Authentication for humans and agents | Industry standard |
| **RBAC** (Role-Based Access Control) | Authorization model | Industry standard for wikis |

### 15.1 Standards Adapted (concepts, not wholesale)

| Standard | What We Take |
|---|---|
| **SLSA** (Supply-chain Levels for Software Artifacts) | Graduated trust levels for content (L0-L3) |
| **W3C Verifiable Credentials** | Agent identity data model (shapes, not full infra) |
| **DITA** (Darwin Information Typing Architecture) | Document type taxonomy (concept, task, reference, decision) |
| **RFC/IETF lifecycle** | Document status lifecycle (draft → active → superseded) |
| **in-toto attestation** | Patterns for pipeline integrity and promotion policies |
| **A2A** (Agent-to-Agent Protocol) | Agent capability advertisement (when stable) |

---

## 16. Prior Art and Influences

| Source | What We Took |
|---|---|
| **Karpathy LLM Wiki Pattern** | Three-layer architecture (sources, wiki, schema). Three operations (ingest, query, lint). Write-back compounding. |
| **PHAT-TOAD Framework** | Steward council protocol, provenance tags, mechanical gates, anti-pattern detection, agent handoff protocol, inspector pattern. Rule 1 (total recursion), Rule 2 (PRD is the memory). |
| **Pippi Knowledge Librarian** | Librarian pipeline (classify→validate→normalize→dedup→write), authority model (standard/insistent/override), progressive classification, vocabulary management, keep-both-linked dedup, fast-track bypass. |
| **Gitea wiki module** | Git bare repo per wiki pattern, WebPath abstraction, service layer design. |

---

## 17. Novel Contributions

No production system combines all of:

1. **Work surface for planning and research** — the wiki IS the brainstorming tool, capturing the birth process of knowledge with dual human/agent input streams, preserving intent, reasoning, and rejected approaches as first-class records minable across projects
2. **Provenance-based trust propagation** with source pinning, checksums, and cascading invalidation
3. **Self-healing pages** that detect when their sources mutate and flag for re-evaluation
4. **Steward agents** that proactively resolve contested data
5. **Challenge mechanism** where agents challenge source of truth with evidence
6. **Dual-surface** (human WYSIWYG + machine MCP) over the same git-backed content
7. **Knowledge supply chain levels** (adapted from SLSA)
8. **Three submission modes** (verbatim / conform / integrate) with mandatory diff preview
9. **Librarian-managed ingestion** with progressive classification and vocabulary management
10. **Recursive architecture** where sources, decisions, vocabulary, and configuration are all wiki pages

---

*End of design specification.*

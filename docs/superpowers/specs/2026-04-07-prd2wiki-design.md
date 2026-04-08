# prd2wiki — Design Specification

**Date:** 2026-04-07
**Status:** Draft
**Version:** 0.1

---

## 1. Purpose

prd2wiki is core infrastructure for agentic development. It provides a persistent, curated, multi-project knowledge backbone where AI agents and humans collaboratively maintain living documents — PRDs, requirements, research, decisions — with full provenance, history, and reasoning.

The system serves as a **source of truth** that agents can read, write, challenge, and reason about. It encourages dialog: agents are expected to challenge the source of truth when they find contradictions, errors, or new information. The truth branch is highly curated; input is open.

### 1.1 What This Is

- A git-backed wiki with WYSIWYG web UI for humans and MCP interface for agents
- A knowledge management system with provenance-based trust, source pinning, and self-healing
- A multi-project, multi-team platform with standard RBAC
- A librarian-managed data pipeline that classifies, validates, normalizes, and indexes all content

### 1.2 What This Is Not

- Not a static document generator
- Not a chat-based knowledge system (no conversation storage)
- Not a vector database with a UI (vectors are a derived index, not the content store)

### 1.3 Design Principles

1. **Familiar, trusted building blocks.** Use established standards (git, markdown, RBAC, MCP, Dublin Core, MADR). Only invent where nothing exists. People should recognize this as a natural evolution of things they already know.

2. **Composable, recursive architecture.** New features stem from a robust set of building blocks adapted via configuration, not rewrites. Everything that can be a wiki page is a wiki page — sources, decisions, vocabulary, trust policies. One implementation, adapters absorb differences (PHAT-TOAD Rule 1).

3. **Git is truth.** All content lives in git as markdown files. Everything else — SQLite index, vector embeddings, computed metadata — is derived, disposable, and rebuildable from git.

4. **Schema is the type system.** Draft branches are dynamically typed (flexible, fast). Truth branch is statically typed (strict, reliable). Nothing merges to truth without full schema compliance.

5. **Provenance is first-class.** Every page records what it was built from, with checksums pinning exact versions. When sources mutate, the system detects it automatically and cascades invalidation.

6. **The librarian never silently changes content.** Every mutation is previewed. Users and agents are always in control.

---

## 2. System Architecture

### 2.1 Components

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
│    │UI  │ │    │Adapters│                        │
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

**Core API** — REST API (standard library `net/http`). Serves both the web UI and sidecars. CRUD for pages, search, provenance queries, branch operations, challenge management.

**Sidecars** — independently deployable processes that speak to the Core API:
- **Web UI**: Milkdown WYSIWYG editor (markdown-native, ProseMirror + remark, MIT). Served as static assets + API calls.
- **MCP Sidecar**: Exposes wiki as MCP resources/tools. Speaks MCP to agents, REST to wiki core. Can be updated/replaced without recompiling the core.
- **Future adapters**: CLI, webhooks, Slack, etc. All attach the same way.

### 2.2 Technology Stack

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

## 3. Content Model

### 3.1 Page Format

Every wiki page is a markdown file (CommonMark + GFM) with structured YAML frontmatter.

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

### 3.2 Document Types

Adapted from DITA topic typing. All types use the same storage, versioning, and provenance model — the `type` field drives rendering and validation rules.

| Type | Purpose |
|---|---|
| `requirement` | Product/system requirements |
| `concept` | Explanatory/background information |
| `task` | How-to procedures |
| `reference` | Lookup information (APIs, schemas, configs) |
| `decision` | Decision records (MADR format) — context, options, decision, consequences |
| `source` | Registered external sources (bibliography entries) |
| `config` | Wiki configuration (trust policies, vocabulary, schemas) |

### 3.3 Structured Blocks

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

### 3.4 Trust Levels (adapted from SLSA)

| Level | Meaning | Requirements |
|---|---|---|
| L0 | Unattributed | Content exists, no provenance metadata |
| L1 | Attributed + cited | Has author, sources listed |
| L2 | Signed + verified | Commit signed, sources independently checked |
| L3 | Reviewed + approved | Went through CR protocol, steward validated provenance chain |

Only L3 content can be merged to the truth branch.

---

## 4. Branching & Truth Model

### 4.1 Branch Structure

```
truth              ← published source of truth (L3 only, schema-conformant)
├── draft/*        ← work in progress (any trust level, flexible schema)
├── challenge/*    ← disputes with evidence
├── ingest/*       ← incoming data from external sources
└── steward/*      ← steward agent work branches
```

### 4.2 Content Flow

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

### 4.3 Change Request (CR) Protocol

Modeled on pull requests, enforced mechanically:

1. Author creates a draft or challenge branch with changes
2. Each changed page must have complete frontmatter (provenance, sources, trust level)
3. Librarian validates: schema compliance, provenance chain intact, no broken references, sources not deprecated
4. Steward agent or human reviewer evaluates the CR
5. If provenance chain traces back to vetted sources without breaks → approval can be automated
6. If new unvetted sources or contradictions exist → requires review
7. Merge to truth only when all gates pass

### 4.4 Challenge Flow

When an agent or human finds contested data:

1. Create `challenge/topic-name` branch
2. On that branch, the contested page gets `status: contested` and a `[!challenge]` block with evidence
3. The challenge references specific sources and explains the contradiction
4. On the truth branch, the affected page gets a contest flag in frontmatter: `contested_by: challenge/topic-name`
5. Truth page content stays unchanged, but the flag is visible to all readers and agents
6. Steward agent picks up contested pages, solicits evidence, resolves
7. Resolution either: merges the challenge (truth updates) or closes it (challenge rejected with reasoning captured as a `[!decision]` block)

### 4.5 Cascading Invalidation

When a source is deprecated or challenged:
1. Index query: "what pages cite this source?"
2. All dependent pages get flagged for re-evaluation
3. Their trust levels may drop (L3 → L1 if a source is no longer verified)
4. Steward agents are notified to investigate
5. When the owning agent updates affected sections, trust restores and healing cascades downstream

---

## 5. Pages as Build Artifacts — Source Pinning and Self-Healing

### 5.1 The Principle

Every page is an **as-built artifact** with a manifest. Like a Docker image with a Dockerfile — you know exactly what went into it, and you can tell when the inputs have changed. The wiki doesn't just version content — it tracks the validity of content against its sources continuously.

### 5.2 Source Pinning

Each source in `provenance.sources` records:
- The reference (wiki page, URL, file path)
- The version/commit at time of use
- A checksum of the source content
- The retrieval date

### 5.3 Staleness Detection

| Source Type | How Pinned | How Staleness Detected |
|---|---|---|
| Wiki page (same project) | Page ID + version + checksum | Git hook on commit — immediate |
| Wiki page (cross-project) | Project + page ID + version + checksum | Periodic poll or webhook between projects |
| External URL | URL + checksum + retrieval date | Periodic fetch and re-checksum |
| Source code file | File path + commit hash + checksum | Git hook on commit to source repo |
| Local file (PDF, doc) | Path + checksum, local copy in `_sources/` | Local copy preserved; staleness if original changes |

### 5.4 Impact Assessment

When staleness is detected, the steward agent:
1. Diffs the old and new versions of the changed source
2. Determines impact:
   - **No impact:** Change doesn't affect the page's claims. Clear the flag, update checksum.
   - **Partial impact:** Some sections still valid, some not. Mark specific sections, explain what's accurate and what's not.
   - **Full invalidation:** Change fundamentally breaks the page's claims. Flag for owner to rebuild.

### 5.5 Self-Deprecation

If a page's sources are thoroughly invalidated, the system auto-sets `status: deprecated` with an explanation and required action.

### 5.6 Local Source Copies

External sources get a local copy when possible, stored in `_sources/` or `_attachments/`. Preserves what the page was built from even if the external source disappears.

---

## 6. Sources as Wiki Pages (Recursive Architecture)

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

## 7. Cross-Project Trust

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

## 8. Access Control

### 8.1 Model

Standard RBAC with hierarchical content scoping. Three layers evaluated top-down:

1. **System role** — what kind of user (admin, editor, viewer, guest)
2. **Project scope** — what can your group do in THIS project
3. **Page override** — does THIS page restrict further (optional allowlist)

### 8.2 Roles

| Role | Capabilities |
|---|---|
| `admin` | Manage users, config, structure, all project operations |
| `editor` | Read, write, propose, challenge, approve CRs |
| `viewer` | Read only |
| `guest` | Access to specifically shared pages only |

### 8.3 Project Scoping

```yaml
# config/projects.yaml
projects:
  project-a:
    admins: [jane@example.com]
    editors: [team-engineering, agent-lint-3@service.internal]
    viewers: ["*"]
```

### 8.4 Page-Level Overrides

Optional, restrict-only:

```yaml
# In page frontmatter
access:
  restrict_to: [team-security, jane@example.com]
```

### 8.5 Authentication

OIDC/OAuth for both humans and agents. Identity provider manages users and groups (Keycloak, Okta, GitHub, etc.). The wiki does not manage user accounts or group membership. Same permission evaluation for API and UI. Tokens are user-scoped.

### 8.6 Quality Gates (Separate from Permissions)

Branch protection rules enforce content quality. A user might have `editor` role but their merge still fails if:
- Schema validation fails
- Provenance chain is broken
- Trust level < L3
- Required reviewers haven't signed off

Quality gates = merge rules, not access rules.

---

## 9. Librarian

### 9.1 Role

The librarian is a core component that sits between the Core API and storage. Every write passes through it. It implements the ingestion pipeline adapted from the Pippi Knowledge Librarian.

### 9.2 Pipeline

```
classify → validate → normalize → deduplicate → cross-reference → write → index
```

### 9.3 Submission Modes

Three modes give explicit control over what the system does with input:

| Mode | UI Label | Librarian Behavior |
|---|---|---|
| **Verbatim** | `Submit [Do Not Mutate]` | Schema validation (flag, don't block). Save with `conformance: pending`. Index. No normalization. |
| **Conform** | `Submit [Correct & Format]` | Validate + normalize spelling/format/tags + dedup check. Return diff for preview. No content changes. |
| **Integrate** | `Submit [Reason & Merge]` | Full pipeline — validate + normalize + cross-reference + reason about placement + suggest merges. Return diff for preview. |

**Mandatory diff preview** for conform and integrate modes. The user/agent sees exactly what changed before confirming. Can accept, reject, or switch to verbatim.

### 9.4 Progressive Classification (adapted from Pippi)

1. **Rule-based** (< 1ms): frontmatter `type` field, structural pattern matching
2. **Vector similarity** (< 10ms): embed and compare against existing pages in LanceDB
3. **LLM escalation** (budgeted): only for `integrate` mode when confidence < 0.7

### 9.5 Vocabulary Management

Canonical terms for tags, relationship predicates, and document types stored in a vocabulary table (SQLite). Fuzzy matching normalizes incoming terms (> 0.85 similarity = normalize to canonical). New terms logged for review.

### 9.6 Deduplication

Keep-both-linked strategy: near-duplicates with conflicting data are linked as potential duplicates, never auto-merged. Steward agents or humans resolve. Merge execution is explicit with per-field resolution.

### 9.7 Schema Enforcement by Branch

| Branch | Schema Enforcement |
|---|---|
| `truth` | Strict — full compliance required, merge blocked if validation fails |
| `draft/*` | Flexible — non-conforming pages saved with `conformance: pending` |
| `challenge/*` | Partial — challenge metadata must conform, challenged content may be in flux |
| `ingest/*` | Flexible input, strict output — ingest steward produces conforming pages |

### 9.8 Fast-Track Bypass

System-internal writes (steward lint results, computed index updates, automated staleness flags) bypass the full pipeline. Only token verification + schema matching.

---

## 10. MCP Sidecar

### 10.1 Resources (read-only context)

- `wiki://project-a/PRD-042` — full page content + frontmatter
- `wiki://project-a/index` — page catalog for a project
- `wiki://project-a/graph` — provenance/dependency graph
- `wiki://project-a/contested` — pages with active challenges

### 10.2 Tools (agent-invocable actions)

- `wiki_search` — full-text + semantic search across projects
- `wiki_read` — read a specific page
- `wiki_propose` — create/update a page on a draft branch (accepts `intent: verbatim|conform|integrate`)
- `wiki_challenge` — raise a challenge against a page with evidence
- `wiki_ingest` — submit external source material for processing
- `wiki_lint` — run provenance/consistency checks
- `wiki_status` — check CR status, challenge status
- `wiki_log` — append to the operation log

### 10.3 Prompts (reusable workflows)

- `review_page` — structured review template
- `ingest_source` — guided source registration workflow

The sidecar is a separate process. Can be written in Go or TypeScript (whatever best supports the MCP SDK). Updated independently from the core.

---

## 11. Steward Agents

Steward agents are standard wiki users with `editor` role. They're not special in the permission model — they just have tooling and instructions that make them proactive.

### 11.1 Lint Steward

Runs periodically or on-commit:
- Validates provenance chains (are all cited sources still active?)
- Detects contradictions between pages
- Flags orphaned pages (no inbound references)
- Checks for deprecated sources with live dependents
- Runs staleness detection on external URLs
- Reports via `wiki_log`, creates challenges when issues found

### 11.2 Resolution Steward

Activated when contested pages exist:
- Solicits evidence from agents and humans
- Evaluates contradicting claims against sources
- Proposes resolution as a draft, creates `[!decision]` blocks with reasoning
- Does NOT auto-merge — submits CR for review

### 11.3 Ingest Steward

Processes incoming data:
- Reads submitted source material
- Creates source pages (registers in bibliography)
- Extracts key information, creates/updates wiki pages
- Cross-references with existing content
- Follows Karpathy pattern: update 10-15 related pages per ingest

All stewards follow PHAT-TOAD's principle: standard agents with wider scope and specific tooling, not exceptions to the system.

---

## 12. Media & Attachments

### 12.1 Storage

Git LFS for binary files. Media is versioned alongside content in the same commit history.

### 12.2 Directory Structure

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

### 12.3 References

Standard relative markdown paths: `![Diagram](roadmap/_attachments/diagram.png)`. No UUIDs, no custom syntax.

### 12.4 File Type Whitelist (enforced by git hook)

- Images: png, jpg, gif, webp, svg
- Documents: pdf
- Audio/Video: mp3, mp4, webm, ogg
- Data: csv, json, yaml, toml
- Code: py, js, ts, go, rs, sh, sql, etc.
- Reject: exe, dll, .DS_Store, node_modules

### 12.5 Size Limits (enforced by git hook)

- Warn > 10 MB
- Reject > 50 MB
- Large video/audio: link externally

---

## 13. Standards Adopted

| Standard | How Used | Maturity |
|---|---|---|
| **MCP** (Model Context Protocol, spec 2025-11-25) | Agent interface — resources, tools, prompts | Production, 97M monthly SDK downloads |
| **CommonMark + GFM** | Document format | Mature, universal |
| **YAML frontmatter** | Document metadata | De facto standard |
| **Dublin Core** (ISO 15836) | Metadata vocabulary (title, creator, date, source) | ISO standard |
| **Conventional Commits** | Structured commit messages | Widely adopted |
| **MADR v4** (Markdown Any Decision Records) | Decision record format | MIT + CC0, v4.0.0 |
| **W3C PROV-DM** | Provenance vocabulary (entity, activity, agent, wasDerivedFrom) | W3C Recommendation |
| **Gitsign/Sigstore** | Identity-based commit signing, keyless via OIDC | Production |
| **Git LFS** | Binary file storage in git | Industry standard |
| **OIDC/OAuth** | Authentication for humans and agents | Industry standard |
| **RBAC** | Authorization model | Industry standard for wikis |

### 13.1 Standards Adapted (concepts, not wholesale)

| Standard | What We Take |
|---|---|
| **SLSA levels** | Graduated trust levels for content (L0-L3) |
| **W3C Verifiable Credentials** | Agent identity data model (shapes, not full infra) |
| **DITA topic typing** | Document type taxonomy (concept, task, reference, decision) |
| **RFC/IETF lifecycle** | Document status lifecycle (draft → active → superseded) |
| **in-toto attestation** | Patterns for pipeline integrity and promotion policies |
| **A2A** (Agent-to-Agent Protocol) | Agent capability advertisement (when stable) |

---

## 14. Prior Art and Influences

| Source | What We Took |
|---|---|
| **Karpathy LLM Wiki Pattern** | Three-layer architecture (sources, wiki, schema). Three operations (ingest, query, lint). Write-back compounding. |
| **PHAT-TOAD Framework** | Steward council protocol, provenance tags, mechanical gates, anti-pattern detection, agent handoff protocol, inspector pattern. Rule 1 (total recursion), Rule 2 (PRD is the memory). |
| **Pippi Knowledge Librarian** | Librarian pipeline (classify→validate→normalize→dedup→write), authority model (standard/insistent/override), progressive classification, vocabulary management, keep-both-linked dedup, fast-track bypass. |
| **Gitea wiki module** | Git bare repo per wiki pattern, WebPath abstraction, service layer design. |

---

## 15. Novel Contributions

No production system combines all of:

1. **Provenance-based trust propagation** with source pinning, checksums, and cascading invalidation
2. **Self-healing pages** that detect when their sources mutate and flag for re-evaluation
3. **Steward agents** that proactively resolve contested data
4. **Challenge mechanism** where agents challenge source of truth with evidence
5. **Dual-surface** (human WYSIWYG + machine MCP) over the same git-backed content
6. **Knowledge supply chain levels** (adapted from SLSA)
7. **Three submission modes** (verbatim / conform / integrate) with mandatory diff preview
8. **Librarian-managed ingestion** with progressive classification and vocabulary management
9. **Recursive architecture** where sources, decisions, vocabulary, and configuration are all wiki pages

---

*End of design specification.*

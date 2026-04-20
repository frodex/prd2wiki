# DESIGN: Unified Identity and Organization — Pages, Projects, Versions, Blobs

**Date:** 2026-04-11 (R7: volatile .link line 2, page_uuid-first search)
**Status:** LOCKED — all decisions made, ready for implementation
**Supersedes:** ae8fa74, 67e0795, 0419b8a, 6f0688c
**Works with:** [Version-Aware Memory Store](/projects/default/pages/c6525ac) (c6525ac), [Storage Architecture](/projects/default/pages/7adabb6) (7adabb6)
**Audit:** [R1](/projects/default/pages/d6eb1d3)–[R7](/projects/default/pages/492ae2f)

## The Principle

**Everything works like a filesystem.** Create, rename, move, delete — at runtime, no restart. The filesystem is the source of truth. Identity survives all operations. Names can change as many times as you want.

## Three Layers

| Layer | What it is | Source of truth for | Addressed by |
|-------|-----------|-------------------|-------------|
| **Tree** | Directory structure on disk | Organization, URLs, display names, access control | Filesystem paths |
| **Data** | Git bare repos + blob store | Page content, history, binary files | UUIDs |
| **Librarian** | LanceDB + BadgerDB | Search, vectors, versions | Record IDs (`mem_`) |

Each layer has its own identity. Pointers connect them. Moving something in one layer doesn't break the others.

### Stable vs Volatile Identifiers

| Identifier | Where | Stable? |
|-----------|-------|---------|
| **Page UUID** | `.link` line 1, frontmatter `id`, git filename | **Yes** — never changes |
| **Project UUID** | `.uuid` line 1, repo name | **Yes** — never changes |
| Display name | `.link` line 3, `.uuid` line 2 | No — change any time |
| URL slug | `.link` filename | No — rename = redirect |
| Tree path | Directory position | No — move = redirect |
| **Librarian `mem_` ID** | `.link` line 2 | **No — changes every edit.** New chain-forward versioning creates a new `mem_` ID per version. `.link` line 2 is updated after each `syncToLibrarian`. |

**`page_uuid` is the stable identifier for lookups.** The librarian's secondary index resolves `page_uuid` → current `mem_` head ID transparently. Callers (search, get, display) should use `page_uuid`, not `mem_` IDs.

---

## Dot-Files in the Tree

| File | Where | What it does | Lines |
|------|-------|-------------|-------|
| `.uuid` | Project root dir | Project identity + display name | 1: UUID (stable), 2: display name |
| `{name}.link` | Any dir | Page pointer | 1: page UUID (stable), 2: current `mem_` ID (volatile — updated each edit), 3: title |
| `.access` | Any dir | Access control (inherits down) | `read:` and `write:` rules |
| `.301` | Old location | Permanent redirect | Target path |
| `.302` | Old location | Temporary redirect | Target path |

---

## Complete Example: What's On Disk

```
/srv/prd2wiki/
│
├── config/
│   └── prd2wiki.yaml                       ← server settings ONLY
│
├── tree/
│   ├── .git/                                ← tracks tree changes
│   ├── .access                              ← global default access rules
│   ├── engineering/
│   │   ├── prd2wiki/
│   │   │   ├── .uuid                        ← project identity + display name
│   │   │   ├── .access                      ← project-level access rules
│   │   │   ├── master-plan.link             ← page pointer
│   │   │   ├── architecture/
│   │   │   │   ├── storage.link
│   │   │   │   ├── embedder.link
│   │   │   │   └── record-model.link
│   │   │   ├── plans/
│   │   │   │   └── phase-plan.link
│   │   │   ├── security/
│   │   │   │   ├── .access                  ← restricted access
│   │   │   │   └── auth-design.link
│   │   │   └── bugs/
│   │   │       └── known-issues.link
│   │   └── svg-terminal/
│   │       ├── .uuid
│   │       └── overview.link
│   └── games/
│       └── battletech/
│           ├── .uuid
│           └── mechlab.link
│
├── data/
│   ├── repos/
│   │   ├── proj_660f9500.git/
│   │   ├── proj_770a8601.git/
│   │   └── proj_880b9702.git/
│   └── blobs/
│       ├── ab/cdef1234567890...
│       └── 9f/8e7d6c5b4a3928...
```

---

## Every File Explained

### config/prd2wiki.yaml

Server settings only. Tells the wiki where to look, not what to find.

```yaml
server:
  addr: "0.0.0.0:8082"
data:
  dir: "./data"
tree:
  dir: "./tree"
librarian:
  socket: "/var/run/pippi-librarian.sock"
logging:
  level: "info"
```

No project list. No page list. No bootstrap. Empty `tree/` = empty wiki.

---

### .uuid (project identity)

**Example:** `tree/engineering/prd2wiki/.uuid`

```
660f9500-f30c-52e5-b827-557766551111
PRD Wiki
```

**Line 1:** Project UUID (v4) — globally unique, never changes.
**Line 2:** Display name — shows in sidebar. Change any time.

**Repo convention:** `data/repos/proj_{first 8 chars of UUID}.git`

---

### {page-name}.link (page pointer)

**Example:** `tree/engineering/prd2wiki/architecture/storage.link`

```
550e8400-e29b-41d4-a716-446655440000
mem_r8k2f1a3
Storage Architecture
```

**Line 1:** Page UUID (v4) — **stable.** Never changes. Points to `pages/550e8400.md` in git. Same value as frontmatter `id` field.
**Line 2:** Current librarian `mem_` ID — **volatile.** Changes on every page edit (new-chain-forward versioning). Updated after each `syncToLibrarian`. Empty on initial creation (filled after first sync or bulk import). Use `page_uuid` for lookups, not this value.
**Line 3:** Display title — shows in sidebar. Change any time.

**The filename is the URL slug.** `storage.link` → URL is `.../storage`. Rename file → URL changes. UUID inside stays the same.

**Why three lines instead of YAML:** Sidebar reads line 3 of every `.link` file. Reading 300 three-line files = microseconds. Parsing 300 YAML files = the 7.6s page list problem we already fixed.

---

### .access (access control) — PROPOSED, NOT YET IMPLEMENTED

**Deferred to OAuth phase.** Convention documented here for future implementation. Format is decided — the auth backend is future work. No identity provider selected yet.

**Rules:**
- `read: *` — anyone can read
- `write: greg, mcp-agent` — only these principals can write
- Rules inherit down the tree. Override at any level.
- Move a folder → access rules move with it.

---

### .301 / .302 (redirects)

When a page or project moves, leave a redirect at the old location. Wiki serves HTTP 301/302. Project redirects append remaining path. Cleanup: manual for now.

---

### Subdirectories (folders)

Organizational only. No `.uuid` needed. Can have `.access`.

---

### Page .md files in git

**Example:** `pages/550e8400.md` inside `data/repos/proj_660f9500.git`

**Flat layout: `pages/{uuid}.md`** — no hash-prefix subdirectories.

```yaml
---
id: "550e8400-e29b-41d4-a716-446655440000"
librarian_id: "mem_r8k2f1a3"
title: "Storage Architecture"
type: reference
status: draft
dc.creator: mcp-agent
dc.created: "2026-04-10"
tags:
  - pippi
  - librarian
  - architecture
---
```

**Filename = UUID.** Git never renames files when pages move in the tree.

**Frontmatter `id` field = .link line 1 = git filename.** All three are the same UUID. The field is called `id` (not `uuid`).

**`librarian_id` in frontmatter** is informational — may be stale (volatile). The authoritative current `mem_` ID is in `.link` line 2 and the librarian's secondary index.

**Does NOT contain:** Tree path, URL, project name. The page doesn't know where it's displayed.

---

### Blob files

Content-addressed binary. SHA-256 hash as filename. Not in git. Permanent URLs. Deduplicated.

---

## How Identity Flows

```
URL: /engineering/prd2wiki/architecture/storage

Tree lookup (stable):
  storage.link        → line 1: page UUID 550e8400 (STABLE)
                         line 2: mem_r8k2f1a3 (volatile — current head)
                         line 3: "Storage Architecture"
  .uuid               → line 1: project UUID 660f9500 (STABLE)

Git lookup (by page UUID):
  proj_660f9500.git → pages/550e8400.md → full content + frontmatter

Librarian lookup (by page UUID, not mem_ ID):
  page_uuid_index["550e8400"] → current mem_ head → version 5, 4 previous, vector

Render:
  title from .link line 3, content from git, version info from librarian, images from blobs
```

**Search returns `page_uuid`.** The wiki resolves `page_uuid` → current tree path for display URL. The `mem_` ID in search results is the current head at query time — it may change if the page is edited between search and click.

---

## Multiple Renames — Nothing Breaks

### Project renamed 4 times

UUID, repo, librarian records — all unchanged at every step. Only tree paths and display names change.

### Page moved 3 times

UUID, git data, librarian chain — all unchanged at every step. Only tree location and URL change. `.link` line 2 (`mem_` ID) is unaffected by moves — it only changes on content edits.

---

## Operations

| Operation | Command | What changes | What doesn't |
|-----------|---------|-------------|-------------|
| Create project | `mkdir` + `.uuid` + `git init --bare` | Tree, disk | Nothing |
| Create page | Write `.md` to git + `.link` to tree | Git, tree | Nothing |
| Move page | `mv {name}.link` + `.302` | Tree | Git, librarian, UUID |
| Move project | `mv` directory + `.302` | Tree | Repo, librarian, UUIDs |
| Rename page slug | `mv {old}.link {new}.link` + `.301` | Tree filename | UUID, git, librarian |
| Edit page | Write to git, `syncToLibrarian` | Git, librarian, **.link line 2** | UUID, tree path, title |
| Change display name | Edit `.uuid` line 2 or `.link` line 3 | Display name | UUID, URL, git |
| Add image | Store in `data/blobs/` + reference in markdown | Blob store, git | Tree |
| Delete page | `rm {name}.link` | Tree | Git (preserved), librarian |

---

## How Versioning, Search, Export Connect

**Versioning:** Librarian stores by `page_uuid`. New-chain-forward: each edit creates a new `mem_` head, old head demoted to superseded. Chain linked by bidirectional pointers. Move the page → `page_uuid` stays → chain intact.

**Search:** Returns `page_uuid` (stable) + current `mem_` ID (volatile snapshot). Wiki resolves `page_uuid` → current tree path for display URL. Always route by `page_uuid`, not `mem_` ID.

**Export:** `tar czf backup.tar.gz tree/ data/` — complete wiki. Restore anywhere. Librarian rebuilt from git. Export tool also includes `schema.d/` and `manifest.json`. See [Export/Import](/projects/default/pages/cec9acb) (cec9acb).

---

## What This Replaces

| Before | After |
|--------|-------|
| Page identity = git path | Page identity = UUID |
| Project identity = config + dir name | Project identity = UUID in `.uuid` |
| Project list = config (restart) | Project list = tree scan (runtime) |
| URLs = `/projects/default/pages/04/20776` | URLs = `/engineering/prd2wiki/storage` |
| Sidebar = parse 300 YAML | Sidebar = read 300 `.link` line 3 |
| Images in git | Content-addressed blob store |
| Image URLs break on move | Blob URLs permanent |
| Move = break everything | Move = `mv` + redirect |
| No access control | `.access` files (proposed) |
| No redirects | `.301`/`.302` files |
| Config has project list | Config has server settings only |

---

## Decisions (formerly Open Questions)

| # | Question | Decision | Rationale |
|---|----------|----------|-----------|
| 1 | UUID format | UUID v4 (36 chars) | Industry standard. Go `google/uuid`. |
| 2 | Legacy URLs | Permanent 301 redirects | Stay forever. Remove manually. |
| 3 | Same page in multiple locations | No | One `.link` per page UUID. |
| 4 | Auto-organize | New pages → project root (flat) | User organizes manually. |
| 5 | Source code tracking | Out of scope | Future work. |
| 6 | Page lookup in git | Flat `pages/{uuid}.md` | No hash-prefix subdirs. |
| 7 | Redirect cleanup | Manual for now | Future: CLI tool. |
| 8 | `.access` implementation | Deferred to OAuth phase | Format documented, backend future. |

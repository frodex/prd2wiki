# FINAL PASS: Implementation Readiness — Security, Codebase Reality, and Execution

**Date:** 2026-04-12  
**Kind:** Pre-implementation adversarial review (published for the record)  
**Canon:** [Master Plan 8634f02](/projects/default/pages/8634f02), [Phase 3 Expansion 56803d5](/projects/default/pages/56803d5), [Version-Aware Memory c6525ac](/projects/default/pages/c6525ac), [Librarian Tools 6ccd407](/projects/default/pages/6ccd407), [Export/Import cec9acb](/projects/default/pages/cec9acb), [Unified Identity de836ff](/projects/default/pages/de836ff), [Pre-flight 92657c7](/projects/default/pages/92657c7)  
**Prior audits:** [R1](/projects/default/pages/d6eb1d3)–[R13](/projects/default/pages/7eafc7b)

---

## Verdict

**Proceed to implementation** — the **wiki canon** is now coherent: Phase 3 is decomposed (3a–3d), R13 gaps are **folded into [8634f02](/projects/default/pages/8634f02)** and **[56803d5](/projects/default/pages/56803d5)** (reserved prefixes, **308** API redirects, **libclient** in 3a.7, **3d stubs** before Phase 5 CLIs, **delete → async `memory_delete` by `page_uuid`** + compactor story, **two MCP schema tracks**).

**Condition:** Success depends on **treating the spec as a migration of two live codebases**, not a greenfield app. The sections below name **real gaps** between **today’s code** and **tomorrow’s spec**, plus **non-negotiable security work** before any production exposure.

---

## 1. Codebase reality (why “build failure” is a process risk, not fate)

### prd2wiki (`/srv/prd2wiki`)

- **API surface today:** `POST/GET/PUT/DELETE /api/projects/{project}/pages/...` (`prd2wiki/internal/api/server.go`) — **no** `/api/tree/...`, **no** catch-all tree router.
- **`SubmitRequest`** (`prd2wiki/internal/librarian/librarian.go`) has **`Project`, `Branch`, `Frontmatter`, `Body`, `Intent`** — **no `PageUUID` / `ProjectUUID`**, **no `syncToLibrarian`**, **no `internal/libclient/`** (confirmed: **no matches** for those symbols in-tree at review time).
- **Implication:** Phases **3a + pre-flight Item 1** are **large mechanical refactors**, not thin wrappers. Schedule risk is **integration surface area**, not documentation quality.

### pippi-librarian (`/srv/pippi-librarian`)

- **`MemoryStore.Store`** (`pippi-librarian/internal/librarian/memory.go`) uses **`namespace` + `key`**, **updates in place** when a row exists — this is **not** [c6525ac](/projects/default/pages/c6525ac) **new-chain-forward** (new row per edit, volatile head `mem_`, `page_uuid` index).
- **Implication:** Phase **2** is a **memory subsystem rewrite** (schema, Lance paths, MCP contract), not a “flip a flag.” Plan effort (~5h) is **optimistic** unless scoped as **incremental PRs** with feature flags.

**Strategy (recommended):**

1. **Preflight item 6** (entity table merge) + **single `pippi_memory` table** before Lance churn multiplies.
2. **Ship `generateMemoryID()` + new-chain-forward `versionAndStore`** behind a **config flag**, dual-write or cutover with tests **before** prd2wiki calls it from `libclient`.
3. **prd2wiki:** land **`SubmitRequest` UUID fields** + **single `submit()`** (preflight Item 1) **before** tree router work so every code path can carry **`page_uuid`**.

---

## 2. Security — trust boundaries and concrete threats

### 2.1 Wiki ↔ Librarian transport

**Spec:** Unix socket + ticket auth. **Code:** `AuthTicketManager` in `pippi-librarian/internal/librarian/auth_tickets.go` issues TTL-bound tickets with scopes.

**Requirements before prod:**

- **Socket permissions:** Only wiki service user + ops; **not** world-readable.
- **Ticket scope:** `memory_store` / `memory_delete` must be **scoped** so a compromised wiki key cannot call **unrelated** tools.
- **No TLS on loopback** is fine **only if** socket FS permissions are correct; document **failure mode** if wiki is misconfigured.

### 2.2 prd2wiki HTTP API keys

`RequireAPIKey` (`prd2wiki/internal/auth/middleware.go`) allows **anonymous** access when `requiredScope == ""`.

**Action:** Audit **every** route after tree migration: **mutating** endpoints (`POST/PUT/DELETE`, blob upload, admin import) must **require** a scope. **Anonymous read** may be OK for public wiki — **explicit product decision**, not accidental default.

### 2.3 Tree router and path handling

**Spec:** Reserved prefixes (`/api`, `/static`, `/blobs`, `/admin`, `/projects`, `/health`, `/debug`). **Threat:** `..`, encoded slashes, `%2e%2e`, Unicode homoglyphs in paths → **path traversal** or **wrong handler**.

**Mitigations:**

- Normalize URL path **before** filesystem walk; **reject** any segment `..` or empty after decode.
- Resolve tree roots to **realpath** once; refuse paths **outside** `tree.dir`.
- **Symlinks:** policy = **ignore** or **ban** under `tree/` for production (or resolve with care).

### 2.4 Blob store (`POST /api/blobs`, `GET /blobs/{hash}`)

**Threats:**

- **Disk exhaustion:** huge uploads → **max body size**, per-request limit, quota.
- **Hash confusion:** non-hex or wrong length → **strict validation** before filesystem join (prevents `../` in hash).
- **XSS via `text/html` sniffing:** serving user content with wrong MIME → **safe content-type** policy (octet-stream fallback for unknown).

### 2.5 Admin import/export (`/admin/*`)

**Threats:**

- **CSRF** on “Export now” / “Upload bundle” if session cookies exist — use **SameSite**, **CSRF token**, or **require API key header** only (no cookie auth for admin).
- **Import tarball:** **zip-slip** / absolute paths in tar — **sanitize** paths on extract; only allow under `target/` + manifest verification ([cec9acb](/projects/default/pages/cec9acb) integrity step).

### 2.6 Two MCP servers (wiki vs librarian)

**Spec:** Separate tools ([56803d5](/projects/default/pages/56803d5) §3c.4). **Threat:** agents or operators **paste wrong tool name** → **wrong addressing** (`page_uuid` vs tree path).

**Mitigation:** Distinct **server names**, **tool prefixes**, and **docs** in [6ccd407](/projects/default/pages/6ccd407) vs wiki MCP schema; integration tests that **refuse** cross-wiring.

### 2.7 Secrets and backups

[cec9acb](/projects/default/pages/cec9acb): **no service keys** in export — **verify** export code **never** adds `data/` keys or env files by accident.

---

## 3. Semantic consistency (post–R13)

| Topic | Status |
|-------|--------|
| Volatile `.link` line 2 | Aligned: [de836ff](/projects/default/pages/de836ff), [8634f02](/projects/default/pages/8634f02), [56803d5](/projects/default/pages/56803d5) |
| New-chain-forward | Spec: [c6525ac](/projects/default/pages/c6525ac) — **code:** not yet |
| Delete orphan policy | Spec: async delete + compactor — **must implement** compactor/reconcile rule |
| API **308** + parallel routes | Documented in [56803d5](/projects/default/pages/56803d5) |
| **3d** stubs before Phase 5 | Documented in [8634f02](/projects/default/pages/8634f02) |
| Librarian `memory_*` vs Wiki `wiki_*` | Explicit in [56803d5](/projects/default/pages/56803d5) and [8634f02](/projects/default/pages/8634f02) |

---

## 4. Implementation strategy (order of battle)

1. **Pre-flight A:** single submit path + `PageUUID`/`ProjectUUID` fields (empty until tree exists).
2. **Pre-flight B:** maintenance window; `.link` + UUID migration; **item 11** blob extraction per [56803d5](/projects/default/pages/56803d5).
3. **Phase 1:** `schema.d/wiki.yaml` **field-for-field** vs [6ccd407](/projects/default/pages/6ccd407).
4. **Phase 2 (pippi):** new-chain-forward + zero-pad IDs + repair; **feature-flag** rollout.
5. **Phase 3a:** scanner → **reserved-prefix router** → discovery → **`internal/libclient/`** + `syncToLibrarian` → CRUD → API **308** → blobs.
6. **Phase 3b–3d:** UI, tools, admin stubs.
7. **Phase 4–5:** deploy + CLIs; **wire** admin to real processes.

**Concurrency:** [c6525ac](/projects/default/pages/c6525ac) assumes **single-writer** mutex for version writes — **enforce** in `MemoryStore` and **do not** call `Store` concurrently for the same `page_uuid` from multiple wiki workers without a queue.

---

## 5. Testing matrix (minimum before “done”)

| Layer | What |
|-------|------|
| Unit | ID generator: boundary `36^11-1` vs `36^11`; padded lex order |
| Unit | Path resolver: `..`, `%2f`, over-long segments |
| Integration | `Store` → demote; crash simulation between Lance ops |
| Integration | Export → import → verify A=B per [cec9acb](/projects/default/pages/cec9acb) |
| E2E | Legacy `/projects/.../pages/{id}` → **301**; API **308** preserves method |
| Security | Blob upload max size; tar extract path trap |
| Load | Stress target from [8634f02](/projects/default/pages/8634f02) (non-contractual but run) |

---

## 6. Risk register (short)

| Risk | Mitigation |
|------|------------|
| Memory rewrite larger than Phase 2 estimate | Feature flags; table merge first; vertical slices |
| Tree router breaks existing API clients | 308 + parallel routes + `Deprecation` header per [56803d5](/projects/default/pages/56803d5) |
| Orphan librarian rows | Async delete + compactor reconcile |
| Admin UI before CLIs | Stubs explicit in plan — no fake “success” in UI |
| File map drift | Update [97a0970](/projects/default/pages/97a0970) when `internal/tree/`, `libclient`, admin routes land |

---

## 7. Sign-off statement

The **documentation set** reviewed here is **fit to drive implementation**. **Build failure** at this point would stem from **underestimating rewrite scope**, **skipping security hardening on new surfaces**, or **merging the two MCP schemas** — all **knowable** and **preventable** with the checklist above.

**This page does not replace code review or penetration testing.** It is the **last wiki-wide pass** before real commits: **semantics, security, codebase honesty, and execution order.**


# Wiki snapshots (offline canon)

Markdown + JSON copies of **default** project pages, pulled from the running wiki **before** a maintenance window or host shutdown.

**Fetched:** 2026-04-12 (LAN API)

**Source:** `http://192.168.22.56:8082/api/projects/default/pages/{page_id}`

| Page ID | Snapshot |
|---------|----------|
| `8634f02` | Master Implementation Plan |
| `92657c7` | Pre-flight cleanup (items 1–12) |
| `de836ff` | Unified Identity |
| `c6525ac` | Version-Aware Memory |
| `6ccd407` | Librarian Tools |
| `56803d5` | Phase 3 Expansion |
| `cec9acb` | Export/Import |
| `97a0970` | File Map |
| `6dbbae9` | Pippi Librarian architecture |
| `13c87ad` | Head-Delete Gap |
| `7d06afa` | Final pass readiness |
| `d6eb1d3` | Audit R1 (blockers entry) |
| `7eafc7b` | Audit R13 |

- `*.md` — body text only (from API `body` field).
- `*.json` — full API response (includes metadata useful for tooling).

**Refresh** (wiki must be up):

```bash
./scripts/fetch-wiki-local.sh
```

Cross-repo constraints remain in `docs/constraints-prd2wiki-pippi.md` (edited in-repo, not only wiki).

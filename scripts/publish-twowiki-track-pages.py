#!/usr/bin/env python3
"""Create PRD wiki pages for twoWiki tracks A–D + index (Tree API POST)."""
import json
import os
import subprocess
import sys
import urllib.request

BASE = "http://192.168.22.56:8082"


def keygen() -> str:
    out = subprocess.check_output(
        ["go", "run", "-mod=mod", "./cmd/prd2wiki-keygen", "-db", "./data/index.db"],
        cwd="/srv/prd2wiki",
        text=True,
    )
    for ln in out.splitlines():
        if ln.strip().startswith("psk_"):
            return ln.split()[-1].strip()
    raise SystemExit("no psk_ key from keygen")


def post_page(key: str, slug: str, title: str, body: str, tags: list[str]) -> None:
    payload = {
        "slug": slug,
        "title": title,
        "type": "plan",
        "status": "draft",
        "tags": tags,
        "body": body,
        "branch": "draft/incoming",
        "intent": "verbatim",
        "author": "svc:twowiki-tracks-bootstrap",
    }
    data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    url = f"{BASE}/api/tree/prd2wiki/pages"
    req = urllib.request.Request(url, data=data, method="POST")
    req.add_header("Content-Type", "application/json; charset=utf-8")
    req.add_header("Authorization", f"Bearer {key}")
    try:
        with urllib.request.urlopen(req) as r:
            r.read()
    except urllib.error.HTTPError as e:  # type: ignore[name-defined]
        body = e.read().decode("utf-8", "replace")
        if e.code == 409 or "exists" in body.lower():
            print("exists", slug, file=sys.stderr)
            return
        raise


def main() -> None:
    key = keygen()
    pages = [
        (
            "plan-twowiki-tracks-a-through-d-index",
            "twoWiki implementation tracks (A–D) — index",
            """# twoWiki implementation tracks (A–D) — index

> **Head:** [prd2wiki As-Built PRD — Index](/prd2wiki/prd2wiki-as-built-prd-index) | **Guide:** [How to Work on This Collection](/prd2wiki/how-to-work-on-this-collection) | **Matrix:** [Comprehensive Test Matrix](/prd2wiki/plan-twowiki-comprehensive-test-matrix)

**Purpose:** One index for the four parallel work tracks. Each track has its own plan + **Progress log** (append-only). Agents advance **A → B → C → D** when blocked; humans clear blockers; agents **re-open A** after D when notes say blockers cleared.

| Track | Page | Scope (one line) |
|-------|------|------------------|
| **A** | [Track A — ticket edit + Milkdown](/prd2wiki/plan-twowiki-track-a-ticket-edit-milkdown) | Browser edit of `body` / `section_*` + rich editor |
| **B** | [Track B — orchestrator history](/prd2wiki/plan-twowiki-track-b-orchestrator-history-json) | Machine-readable ticket change history |
| **C** | [Track C — agent JSON wrappers](/prd2wiki/plan-twowiki-track-c-agent-json-wrappers) | Scripts/MCP around `json/login`, `ticket/get`, `ticket/save` |
| **D** | [Track D — hardening](/prd2wiki/plan-twowiki-track-d-hardening-taxonomy-fts) | Taxonomy validation, FTS, least-privilege query |

**Convention:** Each track page has **## Plan** (stable checklist) and **## Progress log** (newest entry first or append at bottom — use dated `###` headings).
""",
            ["twoWiki", "plan", "tracks", "orchestration"],
        ),
        (
            "plan-twowiki-track-a-ticket-edit-milkdown",
            "twoWiki Track A — ticket edit (`body`) + Milkdown",
            """# twoWiki Track A — ticket edit (`body`) + Milkdown

> **Index:** [Tracks A–D](/prd2wiki/plan-twowiki-tracks-a-through-d-index) | **Matrix:** [Comprehensive Test Matrix](/prd2wiki/plan-twowiki-comprehensive-test-matrix)

## Plan (simple)

1. **Ship `ticket-editpage` TH1** in repo `vendor/twowiki-fossil-skin/`, deployed via `apply_twowiki_skin.py` → `fossil sql`, so `/tktedit/{uuid}` exposes **`body`**, **`section_*`**, **`tags`**, **`page_path`** (not only remark append).
2. **Verify in browser** (logged in): edit `body`, Preview/Submit, reload `/tktview/{uuid}` — content round-trips.
3. **Milkdown (or chosen editor)** bundle + CSP-safe load; wire to the `body` textarea (replace or shadow); keep plain-text fallback.
4. **Optional:** same editor for `section_*` fields or defer with “textarea only” for sections.

## Progress log

### 2026-04-16 — Agent

- Added **`ticket-editpage.th1`** (Fossil default + twoWiki page fields) and wired **`apply_twowiki_skin.py`** to emit `ticket-editpage` into `config`.
- **Deployed** SQL to twowiki host `192.168.20.155` repo `/opt/twowiki/repo.fossil` (`ticket-editpage` length ~6152 bytes in `config`).
- **Blocker for next step:** need **human browser** on `/tktedit/{uuid}` (logged in) to confirm Fossil accepts `submit_ticket` with new fields and does not error on missing `section_*` for old tickets. If TH1 errors, wrap optional fields with `enable_output` / `info exists` guards and redeploy.

**Files:** `vendor/twowiki-fossil-skin/ticket-editpage.th1`, `apply_twowiki_skin.py`
""",
            ["twoWiki", "track-a", "fossil", "milkdown"],
        ),
        (
            "plan-twowiki-track-b-orchestrator-history-json",
            "twoWiki Track B — orchestrator history (JSON)",
            """# twoWiki Track B — orchestrator history (JSON)

> **Index:** [Tracks A–D](/prd2wiki/plan-twowiki-tracks-a-through-d-index)

## Plan (simple)

1. **Pick one path:** (i) parse `tkthistory` HTML, (ii) extend patched `fossil-json` with `json/ticket/history`, (iii) narrow `json/query` behind a **service user** with minimal caps.
2. **Document contract** (request/response shape, auth, pagination) on this page + matrix row.
3. **Implement** smallest slice + one orchestrator consumer test (3 edits on a scratch ticket).

## Progress log

### 2026-04-16 — Agent

- **Current build:** `GET /json/ticket/history` returns **FOSSIL-1102** Unknown subcommand (see matrix API smoke).
- **Blocked** until Track A browser verification completes or product chooses path **(i)/(ii)/(iii)** — no code change in prd2wiki repo for JSON server yet.
""",
            ["twoWiki", "track-b", "fossil", "json"],
        ),
        (
            "plan-twowiki-track-c-agent-json-wrappers",
            "twoWiki Track C — agent JSON wrappers",
            """# twoWiki Track C — agent JSON wrappers

> **Index:** [Tracks A–D](/prd2wiki/plan-twowiki-tracks-a-through-d-index) | **Lab:** repo `AGENTS.md` (twoWiki Fossil lab section) + `.env.twowiki`

## Plan (simple)

1. **`scripts/twowiki-json`** (bash or Go): subcommands `login`, `get`, `save`, `list` reading **`TWOWIKI_*`** from `.env.twowiki`.
2. **Document** examples on this page; keep secrets out of examples (use `$TWOWIKI_FOSSIL_PASSWORD`).
3. **Optional later:** MCP tools `tw_read` / `tw_write` calling the same helper.

## Progress log

### 2026-04-16 — Agent

- **Exists today:** `scripts/twowiki-fossil-auth-smoke.sh` (login-only). **Next:** extend or add `twowiki-json` with `get`/`save` using same Referer + JSON patterns as [Fossil ticket tests plan](/prd2wiki/plan-twowiki-fossil-ticket-tests).
- **Blocked** briefly on naming + error UX — implement after Track B path decision if `save` responses need history correlation.
""",
            ["twoWiki", "track-c", "agents", "scripts"],
        ),
        (
            "plan-twowiki-track-d-hardening-taxonomy-fts",
            "twoWiki Track D — hardening (taxonomy + FTS + gates)",
            """# twoWiki Track D — hardening (taxonomy + FTS + gates)

> **Index:** [Tracks A–D](/prd2wiki/plan-twowiki-tracks-a-through-d-index) | **Taxonomy:** [Document Taxonomy](/prd2wiki/document-taxonomy)

## Plan (simple)

1. **Map** matrix §M2 / schema: which `type`/`status` values are legal vs tracker legacy.
2. **Enforce** on **`/json/ticket/save`** (custom Fossil build, proxy, or post-hook) — smallest reject/validate path.
3. **FTS:** confirm ticket `body`/`section_*` indexed; add report or JSON if gaps.
4. **Service identity** for orchestrator reads instead of broad anonymous `json/query`.

## Progress log

### 2026-04-16 — Agent

- **Not started** in code — depends on Track **B** (query/history strategy) and product rules for taxonomy strictness.
- **Blocked** until Track B decision + examples of invalid payloads to reject.
""",
            ["twoWiki", "track-d", "taxonomy", "fts"],
        ),
    ]
    for slug, title, body, tags in pages:
        post_page(key, slug, title, body, tags)
        print("post", slug)


if __name__ == "__main__":
    main()

#!/bin/bash
# Backfill documents into the wiki git repo with original timestamps.
# This writes directly to the git bare repo, bypassing the API,
# so commit dates match the original file creation times.

set -euo pipefail

REPO="/srv/prd2wiki/data/default.wiki.git"
BRANCH="draft/incoming"
API="http://localhost:8082"

# Helper: commit a file to the wiki repo with a specific date and author
commit_page() {
    local id="$1" file="$2" date="$3" author="$4" message="$5"
    local path="pages/${id}.md"

    export GIT_AUTHOR_DATE="$date"
    export GIT_COMMITTER_DATE="$date"
    export GIT_AUTHOR_NAME="$author"
    export GIT_COMMITTER_NAME="$author"
    export GIT_AUTHOR_EMAIL="${author}@prd2wiki"
    export GIT_COMMITTER_EMAIL="${author}@prd2wiki"

    # Use the Go binary to write (it handles bare repo tree construction)
    # Instead, use the API but we can't set dates via API...
    # So we'll use the API then amend the commit date in git

    local body
    body=$(cat "$file")

    # POST to API
    local resp
    resp=$(curl -s -X POST "${API}/api/projects/default/pages" \
        -H "Content-Type: application/json" \
        -d "$(python3 -c "
import json, sys
body = open('$file').read()
print(json.dumps({
    'id': '$id',
    'title': '',
    'type': 'requirement',
    'body': body,
    'intent': 'verbatim',
    'author': '$author'
}))
")")

    echo "  API: $resp" | head -c 100
    echo

    # Now fix the commit date in git
    cd "$REPO"
    local latest_hash
    latest_hash=$(git log "$BRANCH" -1 --format='%H' -- "$path" 2>/dev/null || echo "")

    if [ -n "$latest_hash" ]; then
        # Rewrite the commit with the correct date
        git filter-branch -f --env-filter "
            if [ \$GIT_COMMIT = $latest_hash ]; then
                export GIT_AUTHOR_DATE='$date'
                export GIT_COMMITTER_DATE='$date'
                export GIT_AUTHOR_NAME='$author'
                export GIT_COMMITTER_NAME='$author'
                export GIT_AUTHOR_EMAIL='${author}@prd2wiki'
                export GIT_COMMITTER_EMAIL='${author}@prd2wiki'
            fi
        " "$BRANCH" 2>/dev/null || true
    fi

    cd /srv/prd2wiki
    unset GIT_AUTHOR_DATE GIT_COMMITTER_DATE GIT_AUTHOR_NAME GIT_COMMITTER_NAME GIT_AUTHOR_EMAIL GIT_COMMITTER_EMAIL
}

echo "=== Backfilling documents with original timestamps ==="
echo ""

# The correct chronological order with interleaved NOTES:

echo "1. Research Journal (2026-04-07 14:46)"
commit_page "JOURNAL-concept" \
    "docs/research/2026-04-07-v0.1-prd-wiki-concept-journal.md" \
    "2026-04-07T14:46:53-05:00" "claude" \
    "research: PRD wiki concept journal v0.1"

echo "2. Design Spec v0.1 (2026-04-07 14:46)"
commit_page "SPEC-prd2wiki-design" \
    "docs/superpowers/specs/2026-04-07-prd2wiki-design.md" \
    "2026-04-07T14:46:53-05:00" "claude" \
    "spec: prd2wiki design v0.1 — initial draft"

echo "3. Design Spec v0.2 (2026-04-07 15:42)"
commit_page "SPEC-prd2wiki-design" \
    "docs/superpowers/specs/2026-04-07-prd2wiki-design-02.md" \
    "2026-04-07T15:42:38-05:00" "claude" \
    "spec: prd2wiki design v0.2 — expanded acronyms, work surface as primary feature"

echo "4. Design Spec v0.3 (2026-04-07 15:58)"
commit_page "SPEC-prd2wiki-design" \
    "docs/superpowers/specs/2026-04-07-prd2wiki-design-03.md" \
    "2026-04-07T15:58:43-05:00" "claude" \
    "spec: prd2wiki design v0.3 — added reference trees"

echo "5. Design Spec v0.4 (2026-04-07 17:32)"
commit_page "SPEC-prd2wiki-design" \
    "docs/superpowers/specs/2026-04-07-prd2wiki-design-04.md" \
    "2026-04-07T17:32:08-05:00" "claude" \
    "spec: prd2wiki design v0.4 — work surface elevated, sidecar API, agent remediation"

echo "6. Design Spec v0.1 NOTES by Greg (2026-04-08 00:35)"
commit_page "SPEC-prd2wiki-design" \
    "docs/superpowers/specs/2026-04-07-prd2wiki-design-NOTES.md" \
    "2026-04-08T00:35:24-05:00" "greg" \
    "notes: greg's review of v0.1 — expand acronyms, work surface is primary"

echo "7. Design Spec v0.3 NOTES by Greg (2026-04-08 00:35)"
commit_page "SPEC-prd2wiki-design" \
    "docs/superpowers/specs/2026-04-07-prd2wiki-design-03-NOTES.md" \
    "2026-04-08T00:35:55-05:00" "greg" \
    "notes: greg's review of v0.3 — agent remediation, self-deprecation on newer docs"

echo "8. Bibliography (2026-04-07 19:25)"
commit_page "REF-bibliography" \
    "docs/bibliography.md" \
    "2026-04-07T19:25:35-05:00" "claude" \
    "reference: project bibliography"

echo "9. Phase 1 Plan (2026-04-07 19:33)"
commit_page "PLAN-phase1-wiki-core" \
    "docs/superpowers/plans/2026-04-07-phase1-wiki-core.md" \
    "2026-04-07T19:33:08-05:00" "claude" \
    "plan: Phase 1 — wiki core implementation"

echo "10. Phase 2 Plan (2026-04-07 20:31)"
commit_page "PLAN-phase2-librarian" \
    "docs/superpowers/plans/2026-04-07-phase2-librarian-vectordb.md" \
    "2026-04-07T20:31:19-05:00" "claude" \
    "plan: Phase 2 — librarian + vector index"

echo "11. Phase 3 Plan (2026-04-07 20:47)"
commit_page "PLAN-phase3-web-ui" \
    "docs/superpowers/plans/2026-04-07-phase3-web-ui.md" \
    "2026-04-07T20:47:28-05:00" "claude" \
    "plan: Phase 3 — web UI"

echo "12. Phase 4 Plan (2026-04-07 21:00)"
commit_page "PLAN-phase4-mcp-sidecar" \
    "docs/superpowers/plans/2026-04-07-phase4-mcp-sidecar.md" \
    "2026-04-07T21:00:26-05:00" "claude" \
    "plan: Phase 4 — MCP sidecar"

echo "13. Phase 5 Plan (2026-04-07 21:11)"
commit_page "PLAN-phase5-steward-agents" \
    "docs/superpowers/plans/2026-04-07-phase5-steward-agents.md" \
    "2026-04-07T21:11:17-05:00" "claude" \
    "plan: Phase 5 — steward agents"

echo "14. UX Bugs Journal (2026-04-08 00:08)"
commit_page "JOURNAL-ux-bugs" \
    "docs/research/2026-04-08-v0.1-ux-bugs-journal.md" \
    "2026-04-08T00:08:14-05:00" "claude" \
    "journal: UX bugs discovered during first user testing"

echo ""
echo "=== Backfill complete ==="
echo "Rebuilding index..."
# Restart to rebuild index
bash /srv/prd2wiki/scripts/restart.sh

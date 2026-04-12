# DESIGN: prd2wiki-migrate — History-Preserving Page Migration

**Date:** 2026-04-12
**Status:** Draft — needs implementation
**Problem:** Pre-flight B migration lost all git history (delete+add instead of rename), broke 302 cross-references, and set wrong dates.

## Why This Is a Feature, Not a Script

Page migration happens every time:
- A backup is imported to a new instance
- Pages move between projects
- The wiki upgrades from old ID format to UUID format
- Data is restored from a backup after a failure

The migration tool must be a reliable, repeatable part of the wiki — not a one-time script that gets thrown away.

## What Went Wrong in Pre-flight B

| Problem | Cause | Impact |
|---------|-------|--------|
| **Git history lost** | Migration did `delete old file` + `add new file` as separate commits. Git sees this as "old file deleted, new file created" — no rename tracking. | Every page shows 1 commit ("phase-b: migrate"). All version history gone. |
| **Cross-references broken** | 302 links like `/projects/default/pages/8634f02` in page content not updated to new UUIDs or tree paths. | Clicking any cross-reference in any page → 404. |
| **Dates wrong** | `dc.created` in frontmatter was already set to "today" by `wiki_propose`. Migration didn't fix them from git history. | All pages appear to have the same creation date. Sort by date useless. |
| **Migration noise in git log** | 2-3 commits per page (migrate, remove old path, move attachment). | 1000+ junk commits burying real edit history. |

## Requirements

### R1: Preserve git history
The rename from `pages/{hash-prefix}/{old-id}.md` → `pages/{uuid}.md` must be a **git rename** (same tree entry, new path) so `git log --follow` traces through the rename. One commit per page, not multiple.

### R2: Update cross-references
Every link in every page that references another page by old ID must be updated to the new identifier (tree path URL or UUID) in the **same commit** as the rename. No broken window.

### R3: Fix dates from git history
`dc.created` must be set from the actual first commit date for that page (using `FirstCommitDate()` from the git package). `dc.modified` set from the latest commit date.

### R4: Single commit per page
Each page migration = one git commit containing: file rename + frontmatter update + cross-reference updates. Clean history, no noise.

### R5: Idempotent
Running the migration twice produces the same result. Already-migrated pages are skipped. Partially-failed migrations can be resumed.

### R6: Testable on a second instance
The tool must work against a **copy** of the data, not the live wiki. Verify the result before cutting over. This is the A=B story from the export/import design.

### R7: Mapping file
The old-ID → UUID mapping must be written to a file (`data/migration-map.json`) so that:
- Cross-references can be updated
- Legacy URL redirects can resolve old IDs
- The verify tool can compare old and new instances

## Design

### Migration map (built first, before any changes)

```json
{
  "pages": {
    "8634f02": {
      "uuid": "8cf3ce55-5643-4506-b85f-32655be186c0",
      "old_path": "pages/86/34f02.md",
      "new_path": "pages/8cf3ce55-5643-4506-b85f-32655be186c0.md",
      "title": "PLAN: prd2wiki Master Implementation Plan",
      "slug": "plan-prd2wiki-master-implementation-plan",
      "tree_path": "prd2wiki/plan-prd2wiki-master-implementation-plan",
      "first_commit": "2026-04-11T02:15:00Z",
      "last_commit": "2026-04-12T01:30:00Z"
    }
  },
  "projects": {
    "default": {
      "uuid": "ad85faa0-55d2-4498-96f6-5338cfc054ac",
      "tree_path": "prd2wiki",
      "display_name": "PRD Wiki"
    }
  }
}
```

### Per-page migration (single commit)

For each page in the mapping:

1. **Read** current content from git at old path
2. **Generate UUID** (or use existing from mapping if resuming)
3. **Update frontmatter:**
   - `id` → UUID
   - `dc.created` → from `FirstCommitDate()` 
   - `dc.modified` → from latest commit date
4. **Update cross-references in body:**
   - `/projects/default/pages/{old-id}` → `/{tree-path}` (preferred) or `/projects/default/pages/{uuid}`
   - `](/projects/default/pages/{old-id})` → `](/{tree-path})`
   - Handle both markdown link formats: `[text](url)` and bare URLs
5. **Git rename:** `git mv pages/{hash-prefix}/{old-id}.md pages/{uuid}.md` — this is the critical difference. go-git's `worktree.Move()` or equivalent that produces a rename entry in the commit, not delete+add.
6. **Commit** with message: `migrate: {old-id} → {uuid} ({title})`

### Cross-reference update (all pages, after all renames)

After all pages are renamed, do a second pass:
- For each page, scan body for any remaining old-ID references
- Replace with tree-path URLs using the migration map
- Commit: `migrate: update cross-references`

This two-pass approach (rename first, then cross-ref) avoids circular dependencies where page A references page B which hasn't been renamed yet.

### .link file creation

After git migration is complete:
- Create `tree/` directory structure with `.uuid` files
- Create `.link` files (line 1 = UUID, line 2 = empty, line 3 = title)
- This is the same as Pre-flight B items 8-10, just done after the git work is clean

### Blob extraction

Same as item 11 — extract `_attachments/` to `data/blobs/`, update markdown refs. Can happen in the same commit as the page rename.

## CLI

```bash
# Build mapping (dry run — no changes)
prd2wiki-migrate --config config/prd2wiki.yaml --plan

# Execute migration on a COPY
cp -a data/ /tmp/migration-test/
prd2wiki-migrate --data /tmp/migration-test/ --execute

# Verify against original
prd2wiki-verify --source data/ --target /tmp/migration-test/

# If verify passes, swap
mv data/ data.pre-migration/
mv /tmp/migration-test/ data/
```

## Implementation

```
cmd/prd2wiki-migrate/main.go          — CLI entry point
internal/migrate/
    plan.go                            — build migration map (scan all pages, generate UUIDs, find dates)
    execute.go                         — per-page rename + frontmatter + cross-ref
    crossref.go                        — find and replace old-ID references in markdown
    tree.go                            — create tree/ directory, .uuid, .link files
    blobs.go                           — extract attachments to blob store
    verify.go                          — compare old and new repos (history preserved, content matches)
```

## go-git Rename

The critical operation. go-git's worktree doesn't support bare repos directly. Options:

1. **Clone bare → worktree, rename, push back** — works but slow for large repos
2. **Build new tree objects directly** — manipulate git tree entries to rename the path, preserving the blob hash. This is what the migration script attempted but got wrong (it did delete+add instead of rename).
3. **Use `object.TreeEntry` manipulation** — read the tree, find the entry, change its name, write new tree, commit. The blob SHA stays the same, so git recognizes it as a rename.

Option 3 is correct. The key: the **blob hash must be identical** before and after. If you read the file, modify frontmatter, and write back, the blob hash changes and git won't track it as a rename. So:

**Step 1:** Rename the file (same blob) → git sees rename
**Step 2:** Update content (frontmatter, cross-refs) → git sees content change

Both in the same commit. Git's rename detection looks at the tree diff — if a path disappears and a new path appears with >50% similar content, it's a rename. Since we're only changing frontmatter (small % of content), this should always be detected as a rename.

Actually, the safest approach: **two commits per page.**
1. First commit: pure `git mv` (rename only, no content change) → guaranteed rename detection
2. Second commit: update frontmatter + cross-refs → content change on the new path

Then `git log --follow pages/{uuid}.md` traces through the rename to the full original history.

## Test Plan

1. Copy a repo to /tmp
2. Run migration on the copy
3. Verify: `git log --follow pages/{uuid}.md` shows full history (not just migration commit)
4. Verify: all cross-references in page content resolve
5. Verify: `dc.created` matches `FirstCommitDate()`
6. Verify: no broken links (grep for old-ID patterns)
7. Verify: blob store has all attachments
8. Verify: wiki starts and serves from migrated data

## Restore Plan

The backup at `data.phase-b-backup-20260412010131/` has the original repos. To restore:

```bash
# Stop wiki
pkill -f prd2wiki-fast

# Move migrated data aside
mv /srv/prd2wiki/data/repos /srv/prd2wiki/data/repos.failed-migration

# Restore originals
for repo in /srv/prd2wiki/data.phase-b-backup-20260412010131/*.wiki.git; do
    name=$(basename "$repo")
    cp -a "$repo" "/srv/prd2wiki/data/$name"
done

# Remove stale index (will rebuild on startup)
rm -f /srv/prd2wiki/data/index.db*

# Remove migrated tree (will be recreated by proper migration)
rm -rf /srv/prd2wiki/tree/

# Restart wiki with old config (needs projects: list back)
# Edit config/prd2wiki.yaml to restore projects: [default, svg-terminal, battletech, phat-toad-with-trails]
```

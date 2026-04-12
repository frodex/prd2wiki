# DESIGN: prd2wiki-migrate — History-Preserving Page Migration

**Date:** 2026-04-12
**Status:** Working prototype — tested on live data, needs hardening into production tool
**Code:** `cmd/prd2wiki-migrate/main.go`, `internal/migrate/`

## What This Tool Does

Migrates wiki pages from old hash-prefix IDs (`pages/86/34f02.md`) to UUID-based flat paths (`pages/{uuid}.md`), preserving full git history, fixing cross-references, correcting dates, and creating the tree directory structure.

This is a **feature of the wiki**, not a one-time script. It runs every time:
- A backup is imported to a new instance
- The wiki upgrades ID format
- Data is restored after failure
- Pages are bulk-reorganized

## What We Learned (Failure History)

### Attempt 1: Pre-flight B (FAILED)

The implementing agent wrote `scripts/run-phase-b.sh` and `cmd/prd2wiki-migrate-phaseb/main.go`. It ran against live data.

**Failures:**

| Problem | Cause | How we found it |
|---------|-------|-----------------|
| **Git history destroyed** | Used delete + add (two commits) instead of rename. `git log` on new path showed only 1 commit — the migration commit. 28 real commits for the master plan page, invisible. | User checked History page — showed single entry. |
| **All timestamps identical** | EditCache reads latest git commit per page. Latest commit for every page = migration commit. Page list showed same date for all 211 pages. Sort by date useless. | User tried sorting page list — everything had same timestamp. |
| **Cross-references broken (302 links)** | Page content has links like `/projects/default/pages/8634f02`. Migration renamed IDs to UUIDs but didn't update links in content. | User clicked a cross-reference — 404. |
| **Multiple commits per page** | Separate commits for rename, delete old path, move attachment. 1000+ junk commits. | Git log showed "phase-b: migrate" / "phase-b: remove old" / "phase-b: move attachment" spam. |
| **No test copy** | Ran against live `data/`, not a copy. Couldn't roll back without backup. | Wiki was broken in production. Had to restore from backup. |
| **Repo symlinks missing** | Tree scanner expects `data/repos/proj_{uuid8}.git`. Migration created tree but didn't create repo symlinks. Wiki failed to start. | Error: "could not map UUID prefix to a wiki repo." |
| **Wiki stopped working** | All of the above combined. Old page list slow (90s), tree routes 404. | User couldn't access any project. |

### Attempt 2: prd2wiki-migrate (SUCCEEDED)

Built `cmd/prd2wiki-migrate/` with `internal/migrate/`. Tested against `/tmp/migration-test/` first.

**What worked:**

| Fix | How |
|-----|-----|
| **History preserved** | Single commit per page. Content change (frontmatter + cross-refs) is >50% similar to original, so git's rename detection traces through. `git log --follow` shows 28+ commits. |
| **Cross-references updated** | Second pass replaces all `/projects/{project}/pages/{old-id}` with `/{tree-path}` using the migration map. Zero old-style links remaining. |
| **Dates from git** | `dc.created` set from `FirstCommitDate()` (actual first git commit, not wiki_propose "today"). |
| **Tested on copy first** | `cp -a data/ /tmp/migration-test/` → run migration → verify → then run on live data. |
| **Migration map saved** | `data/migration-map.json` maps old-ID → UUID/path/slug/dates. Used by EditCache for history following. |

**What still needed manual work:**

| Gap | Manual fix applied |
|-----|-------------------|
| **Repo symlinks** | `ln -sf ../default.wiki.git data/repos/proj_{uuid8}.git` — not automated by the tool |
| **EditCache migration skip** | Separate code fix: `pickLastNonMigrateCommit()` skips "migrate:" commits |
| **History follow** | Separate code fix: `LoadMigrationAliases()` reads migration map, passes old paths to `PageHistoryAllBranches()` |
| **Rebuild binary** | Had to `go build` and restart wiki after merging code fixes |

## Anti-Patterns Discovered

### 1. Never migrate in place
The first attempt ran against live data. When it broke, the wiki was down and we had to restore from a backup that happened to exist. **Always copy first, verify, then swap.**

### 2. Never delete+add when you mean rename
go-git doesn't have `git mv` for bare repos. The temptation is to delete the old file and add a new one. Git sees this as two unrelated operations — history is severed. The correct approach: write the new file with similar content (>50% match) so git's rename detection kicks in. Or manipulate tree objects directly to preserve the blob hash.

### 3. Test history preservation explicitly
After migration, run `git log --follow` on the new path. If it only shows the migration commit, the rename wasn't detected. This should be an automated check in the tool, not something the user discovers.

### 4. Migration commits poison EditCache
Any new git commit becomes the "latest" for that page. EditCache, sort-by-date, and "recently edited" all break. The fix (skip "migrate:" commits) works but is a bandaid — the real solution is to not create migration commits that look like edits. Consider: set the migration commit's author date to the original last-edit date, so even without the skip logic, the timestamp is correct.

### 5. Cross-references are content, not metadata
Renaming a page ID without updating links in other pages creates 404s. The migration tool must scan ALL pages for references to ANY page being migrated, not just the page being renamed. This requires building the full mapping first, then doing a cross-reference pass.

### 6. Two wikis for testing
The plan discussed A=B verification with two instances. We should have done that. Future: `prd2wiki-migrate --data /tmp/copy --tree /tmp/copy-tree` → `prd2wiki-verify --source data/ --target /tmp/copy/` → if pass, swap.

### 7. Repo naming must be part of the migration
The tree scanner maps `.uuid` → `data/repos/proj_{uuid8}.git`. If the migration creates `.uuid` files but doesn't create repo symlinks, the wiki can't start. The migration tool must handle the full chain: git content → repo paths → tree files.

## Current Architecture

```
cmd/prd2wiki-migrate/main.go     CLI entry: --plan (dry run) or --execute
internal/migrate/
    plan.go                       Scan repos, build old-ID→UUID mapping, get git dates
    execute.go                    Per-page: write new file, delete old, update cross-refs, create tree

internal/git/
    migration_map.go              LoadMigrationAliases: read mapping, wire to history
    history.go                    PageHistory/AllBranches with aliasPaths for follow-through

internal/web/
    editcache.go                  Skip "migrate:" commits, use migrationAliases for old paths
    handler.go                    aliasPathsFor() used in view, edit, diff, history
```

## What the Tool Must Do (Production Requirements)

### Must have
- [ ] **Build mapping first** — scan all pages, generate UUIDs, find dates, find cross-refs. No changes yet.
- [ ] **Single commit per page** — write new file + delete old in one commit. Content similarity >50% for rename detection.
- [ ] **Update cross-references** — replace all old-ID links with tree-path URLs across ALL pages.
- [ ] **Fix dc.created from git** — `FirstCommitDate()`, not frontmatter.
- [ ] **Create repo symlinks** — `data/repos/proj_{uuid8}.git → ../{name}.wiki.git`
- [ ] **Create tree directory** — `.uuid` files, `.link` files via `tree.WriteProjectUUIDFile`/`tree.WriteLinkFile`
- [ ] **Save migration map** — `data/migration-map.json` for history following
- [ ] **Verify after migration** — automated `git log --follow` check on every page
- [ ] **Idempotent** — skip already-migrated pages. Resume partial migrations.
- [ ] **Work on copies** — `--data` flag points to a copy, not live data

### Should have
- [ ] **Blob extraction** — move `_attachments/` to `data/blobs/{sha256}`, rewrite markdown refs
- [ ] **Author date preservation** — set migration commit's author date to original last-edit date so EditCache doesn't need the "migrate:" skip
- [ ] **Dry run with diff** — show what would change without changing anything
- [ ] **Progress reporting** — "migrating page 47/211: master-plan..."
- [ ] **Validation** — check for duplicate slugs, empty titles, broken frontmatter before migrating

### Won't need (at least not now)
- Cross-project page moves (different git repos)
- Incremental migration (some pages old format, some new)
- Rollback to old format

## CLI

```bash
# Dry run — build plan, show what would happen
prd2wiki-migrate --data ./data --tree ./tree --plan

# Save plan for review
prd2wiki-migrate --data ./data --tree ./tree --plan --plan-file migration-plan.json

# Execute against a COPY
cp -a data/ /tmp/migration-test/
prd2wiki-migrate --data /tmp/migration-test --tree /tmp/migration-test-tree --execute

# Verify history preserved
prd2wiki-migrate --data /tmp/migration-test --verify

# If good, swap live data
pkill -f prd2wiki-fast
mv data/ data.pre-migration/
mv /tmp/migration-test/ data/
mv /tmp/migration-test-tree/ tree/
# Restart wiki
```

## Project Configuration

Projects are configured in the CLI (hardcoded for now, should be a config file):

```go
projects := []migrate.ProjectConfig{
    {OldName: "default", TreePath: "prd2wiki", DisplayName: "PRD Wiki"},
    {OldName: "svg-terminal", TreePath: "svg-terminal", DisplayName: "SVG Terminal"},
    {OldName: "battletech", TreePath: "games/battletech", DisplayName: "BattleTech"},
    {OldName: "phat-toad-with-trails", TreePath: "phat-toad", DisplayName: "PHAT-TOAD"},
}
```

## Dependencies on Other Code

| Component | What migration needs from it |
|-----------|----------------------------|
| `tree.WriteProjectUUIDFile` | Create .uuid files |
| `tree.WriteLinkFile` | Create .link files |
| `tree.UniqueSlug` | Deduplicate slugs within a project |
| `git.OpenRepoAt` | Open bare repos at arbitrary paths |
| `git.Repo.WritePageWithMeta` | Write migrated page content |
| `git.Repo.DeletePage` | Remove old path |
| `git.Repo.FirstCommitDate` | Get real creation date |
| `git.Repo.PageHistoryAllBranches` | Get real last-edit date |
| `git.Repo.ListPages` | Scan for pages to migrate |
| `schema.Parse` | Read frontmatter |
| `git.LoadMigrationAliases` | Read mapping for history following (post-migration) |

## Test Procedure

1. `cp -a data/ /tmp/test-migrate/`
2. `go run ./cmd/prd2wiki-migrate/ --data /tmp/test-migrate --tree /tmp/test-tree --execute`
3. For each project's repo:
   - `git log --follow --oneline {branch} -- pages/{uuid}.md` shows full history (not just migration commit)
   - `grep -c '/projects/default/pages/' pages/{uuid}.md` = 0 (no old-style refs)
4. `ls /tmp/test-tree/*/.uuid` — all projects have identity files
5. `find /tmp/test-tree -name '*.link' | wc -l` = total page count
6. Start wiki with `--data /tmp/test-migrate --tree /tmp/test-tree` — pages render, history works, timestamps correct

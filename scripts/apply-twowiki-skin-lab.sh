#!/usr/bin/env bash
# Apply vendor/twowiki-fossil-skin to the twoWiki lab Fossil repo (LAN).
# Default: **style-only** (merged css + default-skin) — does not overwrite footer/tickets/CSP.
# Css + footer (report JS, preserves header): TWOWIKI_SKIN_LAB_FOOTER=1 ./scripts/apply-twowiki-skin-lab.sh
# Full skin from checkout: TWOWIKI_SKIN_LAB_FULL=1 ./scripts/apply-twowiki-skin-lab.sh
# Requires: SSH key to host; Python 3; repo path on host below.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOST="${TWOWIKI_SKIN_SSH:-root@192.168.20.155}"
FOSSIL_DB="${TWOWIKI_FOSSIL_DB:-/opt/twowiki/repo.fossil}"
APPLY=(python3 "$REPO_ROOT/vendor/twowiki-fossil-skin/apply_twowiki_skin.py")
if [[ "${TWOWIKI_SKIN_LAB_FULL:-}" == "1" ]]; then
  APPLY+=(--full-skin --confirm-full)
elif [[ "${TWOWIKI_SKIN_LAB_FOOTER:-}" == "1" ]]; then
  APPLY+=(--with-footer --confirm-footer)
fi
exec "${APPLY[@]}" | ssh "$HOST" "fossil sql -R $FOSSIL_DB"

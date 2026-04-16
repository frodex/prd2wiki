#!/usr/bin/env bash
# Apply vendor/twowiki-fossil-skin to the twoWiki lab Fossil repo (LAN).
# Requires: SSH key to root@192.168.20.155; Python 3; repo path on host below.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOST="${TWOWIKI_SKIN_SSH:-root@192.168.20.155}"
FOSSIL_DB="${TWOWIKI_FOSSIL_DB:-/opt/twowiki/repo.fossil}"
exec python3 "$REPO_ROOT/vendor/twowiki-fossil-skin/apply_twowiki_skin.py" \
  | ssh "$HOST" "fossil sql -R $FOSSIL_DB"

#!/usr/bin/env bash
# Pull plan/architecture pages from the LAN wiki into docs/wiki-local/.
# Requires: curl, jq. Wiki must be running.

set -euo pipefail
BASE="${WIKI_BASE:-http://192.168.22.56:8082}"
OUT="$(dirname "$0")/../docs/wiki-local"
mkdir -p "$OUT"

IDS=(
  8634f02 92657c7 de836ff c6525ac 6ccd407 56803d5 cec9acb 97a0970 6dbbae9
  13c87ad 7d06afa d6eb1d3 7eafc7b
)

for id in "${IDS[@]}"; do
  url="$BASE/api/projects/default/pages/$id"
  echo "GET $url"
  curl -sfS "$url" -o "$OUT/${id}.json"
  jq -r '.body // empty' "$OUT/${id}.json" >"$OUT/${id}.md"
done

echo "Done. Output: $OUT"

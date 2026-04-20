#!/bin/bash
# Build the wiki server binary into bin/prd2wiki.staging (does not touch the running process).
# Run this anytime before your maintenance window. At the window, run:
#   ./scripts/restart-prd2wiki-quick.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p bin
go build -mod=mod -o bin/prd2wiki.staging ./cmd/prd2wiki
ls -la bin/prd2wiki.staging
echo ""
echo "Staged: $(realpath bin/prd2wiki.staging)"
echo "When ready: ./scripts/restart-prd2wiki-quick.sh   (swap binary + restart; no compile)"

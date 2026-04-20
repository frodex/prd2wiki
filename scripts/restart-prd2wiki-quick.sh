#!/bin/bash
# Install staged binary and restart prd2wiki only. No compile — use after stage-prd2wiki-build.sh.
# Stops the listener on :8082, starts ./bin/prd2wiki -config config/prd2wiki.yaml
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
STAGING="bin/prd2wiki.staging"
if [[ ! -f "$STAGING" ]]; then
	echo "Missing $STAGING — run: ./scripts/stage-prd2wiki-build.sh" >&2
	exit 1
fi
install -m 0755 "$STAGING" bin/prd2wiki

PID=""
if command -v ss >/dev/null 2>&1; then
	PID=$(ss -tlnp 2>/dev/null | grep ':8082' | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | head -1)
fi
if [[ -n "${PID:-}" ]]; then
	echo "Stopping prd2wiki pid=$PID"
	kill "$PID" 2>/dev/null || true
	sleep 2
fi

nohup ./bin/prd2wiki -config config/prd2wiki.yaml >> /tmp/prd2wiki.log 2>&1 &
echo "Started prd2wiki (logging to /tmp/prd2wiki.log); waiting for HTTP..."
for _ in $(seq 1 45); do
	if curl -sf -o /dev/null "http://127.0.0.1:8082/" 2>/dev/null; then
		echo "OK — listening on :8082"
		exit 0
	fi
	sleep 1
done
echo "FAILED — not responding on :8082; tail /tmp/prd2wiki.log" >&2
tail -20 /tmp/prd2wiki.log >&2 || true
exit 1

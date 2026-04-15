#!/usr/bin/env bash
# Authenticated Fossil JSON smoke: json/login → authToken (no ticket mutations).
# Loads `.env.twowiki` from repo root if present. See docs/twowiki-lab.env.example.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENVF="$ROOT/.env.twowiki"
if [[ -f "$ENVF" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENVF"
  set +a
fi

FOSSIL="${TWOWIKI_FOSSIL_URL:-http://192.168.20.155:8083}"
REF="${FOSSIL}/"
USER="${TWOWIKI_FOSSIL_USER:-root}"
PW="${TWOWIKI_FOSSIL_PASSWORD:-}"

if [[ -z "$PW" ]]; then
  echo "twowiki-fossil-auth-smoke: TWOWIKI_FOSSIL_PASSWORD not set." >&2
  echo "  Copy docs/twowiki-lab.env.example to .env.twowiki, fill the password, chmod 600 .env.twowiki" >&2
  exit 77
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

curl -sS -G \
  -H "Referer: ${REF}" \
  --data-urlencode "name=${USER}" \
  --data-urlencode "p=${PW}" \
  "${FOSSIL}/json/login" >"$tmp"

python3 -c 'import json,sys
path=sys.argv[1]
with open(path,encoding="utf-8") as f:
    d=json.load(f)
code=d.get("resultCode")
if code and code!="OK":
    sys.stderr.write("LOGIN_FAIL %s %s\n"%(code,d.get("resultText","")))
    sys.exit(1)
t=(d.get("payload") or {}).get("authToken")
if not t or not isinstance(t,str):
    sys.stderr.write("LOGIN_FAIL no payload.authToken\n")
    sys.exit(1)
print("AUTH_SMOKE_PASS authToken_chars=",len(t))' "$tmp"

echo "twowiki-fossil-auth-smoke: OK (login returned authToken; length printed above)"

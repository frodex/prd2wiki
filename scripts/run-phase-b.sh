#!/usr/bin/env bash
# Pre-flight B: items 8 → 10 → 9 (Go) → 9b → 11
# Wiki must be STOPPED. Run from repo root or pass WIKI_ROOT.

set -euo pipefail
WIKI_ROOT="${WIKI_ROOT:-/srv/prd2wiki}"
DATA="$WIKI_ROOT/data"
TREE="$WIKI_ROOT/tree"

uuid8() {
	# First 8 hex chars of UUID (segment before first hyphen)
	echo "$1" | cut -d- -f1
}

gen_uuid() {
	if command -v uuidgen >/dev/null 2>&1; then
		uuidgen
	elif [[ -r /proc/sys/kernel/random/uuid ]]; then
		cat /proc/sys/kernel/random/uuid
	else
		python3 -c 'import uuid; print(uuid.uuid4())'
	fi
}

echo "=== Item 8: tree/.uuid ==="
mkdir -p "$TREE/prd2wiki" "$TREE/svg-terminal" "$TREE/games/battletech" "$TREE/phat-toad"

U_DEFAULT=$(gen_uuid)
U_SVG=$(gen_uuid)
U_BT=$(gen_uuid)
U_PHAT=$(gen_uuid)

printf '%s\n%s\n' "$U_DEFAULT" "PRD Wiki" >"$TREE/prd2wiki/.uuid"
printf '%s\n%s\n' "$U_SVG" "SVG Terminal" >"$TREE/svg-terminal/.uuid"
printf '%s\n%s\n' "$U_BT" "BattleTech" >"$TREE/games/battletech/.uuid"
printf '%s\n%s\n' "$U_PHAT" "PHAT-TOAD" >"$TREE/phat-toad/.uuid"

cat >"$DATA/migration-manifest.json" <<EOF
{
  "wiki_root": "$WIKI_ROOT",
  "data_dir": "$DATA",
  "projects": {
    "default": {"uuid": "$U_DEFAULT", "tree": "tree/prd2wiki", "display": "PRD Wiki"},
    "svg-terminal": {"uuid": "$U_SVG", "tree": "tree/svg-terminal", "display": "SVG Terminal"},
    "battletech": {"uuid": "$U_BT", "tree": "tree/games/battletech", "display": "BattleTech"},
    "phat-toad-with-trails": {"uuid": "$U_PHAT", "tree": "tree/phat-toad", "display": "PHAT-TOAD"}
  }
}
EOF
echo "Wrote $DATA/migration-manifest.json"

echo "=== Backup bare repos (tar) ==="
BK="$WIKI_ROOT/data.phase-b-backup-$(date +%Y%m%d%H%M%S)"
mkdir -p "$BK"
for p in default svg-terminal battletech phat-toad-with-trails; do
	if [[ -d "$DATA/${p}.wiki.git" && ! -L "$DATA/${p}.wiki.git" ]]; then
		cp -a "$DATA/${p}.wiki.git" "$BK/${p}.wiki.git"
		echo "Backed up ${p}.wiki.git"
	fi
done

echo "=== Item 10: move repos + symlinks ==="
mkdir -p "$DATA/repos"
for pair in "default:$U_DEFAULT" "svg-terminal:$U_SVG" "battletech:$U_BT" "phat-toad-with-trails:$U_PHAT"; do
	name="${pair%%:*}"
	uid="${pair#*:}"
	pre="$(uuid8 "$uid")"
	src="$DATA/${name}.wiki.git"
	dst="$DATA/repos/proj_${pre}.git"
	if [[ -L "$src" ]]; then
		echo "skip $name (already symlink)"
		continue
	fi
	if [[ ! -d "$src" ]]; then
		echo "skip $name (no repo)"
		continue
	fi
	mv "$src" "$dst"
	ln -sfn "repos/proj_${pre}.git" "$src"
	echo "mv $src -> $dst ; symlink $name.wiki.git"
	git -C "$dst" log --oneline -1 --all
done

echo "=== Item 9: migrate pages + .link files ==="
cd "$WIKI_ROOT"
go run ./cmd/prd2wiki-migrate-phaseb -manifest "$DATA/migration-manifest.json"

echo "=== Item 9b: remove SQLite index (rebuilt on startup) ==="
rm -f "$DATA/index.db" "$DATA/index.db-shm" "$DATA/index.db-wal" || true

echo "=== Item 11: blob store + rewrite markdown ==="
mkdir -p "$DATA/blobs"
go run ./cmd/prd2wiki-extract-blobs -manifest "$DATA/migration-manifest.json"

echo "=== Done. Start wiki: systemd or prd2wiki binary ==="
echo "Backup copy at: $BK"

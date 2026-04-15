#!/usr/bin/env python3
"""Append a markdown block to a PRD wiki page (Tree API PUT). Usage:
   wiki-append-progress.py <slug> <append.md"""
import json
import subprocess
import sys
import urllib.request

BASE = "http://192.168.22.56:8082"


def keygen() -> str:
    out = subprocess.check_output(
        ["go", "run", "-mod=mod", "./cmd/prd2wiki-keygen", "-db", "./data/index.db"],
        cwd="/srv/prd2wiki",
        text=True,
    )
    for ln in out.splitlines():
        if ln.strip().startswith("psk_"):
            return ln.split()[-1].strip()
    raise SystemExit("no psk_ key from keygen")


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: wiki-append-progress.py <slug> <append.md (stdin)")
    slug = sys.argv[1]
    chunk = sys.stdin.read()
    if not chunk.strip():
        raise SystemExit("empty stdin")
    key = keygen()
    get_url = f"{BASE}/api/tree/prd2wiki/{slug}"
    with urllib.request.urlopen(get_url) as r:
        page = json.loads(r.read().decode("utf-8"))
    body = page.get("body") or ""
    new_body = body.rstrip() + "\n\n" + chunk.lstrip("\n")
    payload = {
        "title": page["title"],
        "type": page.get("type") or "plan",
        "status": page.get("status") or "draft",
        "body": new_body,
        "tags": page.get("tags") or [],
        "branch": page.get("branch") or "draft/incoming",
        "intent": "verbatim",
        "author": "svc:twowiki-tracks-bootstrap",
    }
    data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(
        f"{BASE}/api/tree/prd2wiki/{slug}",
        data=data,
        method="PUT",
    )
    req.add_header("Content-Type", "application/json; charset=utf-8")
    req.add_header("Authorization", f"Bearer {key}")
    with urllib.request.urlopen(req) as r:
        out = r.read().decode("utf-8", "replace")
    print(out)


if __name__ == "__main__":
    main()

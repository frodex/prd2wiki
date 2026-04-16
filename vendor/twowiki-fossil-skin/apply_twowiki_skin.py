#!/usr/bin/env python3
"""Emit SQL to apply twoWiki Fossil skin overlay. Pipe to: fossil sql -R /path/to/repo.fossil

**Default (no flags): style-only** — updates only merged ``config.css`` and ``default-skin``.
Does **not** touch header, footer, ticket TH1, CSP, details, or js. Use that for palette/layout
changes without risking core site behavior.

**Full skin** requires explicit flags (see below). Agents and automation should use the default.

Layout (full mode only — files merged into repo workflow):
  - ``lovable_01a/`` — Fossil skin package (css.txt, header.txt, details.txt, js.txt)
  - ``twowiki-fossil-th1-append.css`` — structural CSS
  - ``one-line-menu-ticket-tags-02a/css.txt`` — design layer (v8+) when present; else ``01a/…/v6.css``. If using ``02a/css.txt``, only the **sortable thead** block is still appended from ``01a/…/v6.css`` when missing in ``02a``.
  - ``footer.th1``, ``ticket-viewpage.th1``, ``ticket-editpage.th1``

``config.mainmenu`` is never emitted — site-specific.

Usage:
  python3 apply_twowiki_skin.py                    # style-only (safe default)
  python3 apply_twowiki_skin.py --full-skin --confirm-full   # all skin keys from checkout
  python3 apply_twowiki_skin.py --help
"""
from __future__ import annotations

import argparse
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
SKIN_PKG = os.path.join(HERE, "lovable_01a")


def esc(s: str) -> str:
    return "'" + s.replace("'", "''") + "'"


def read_skin_file(rel: str) -> str:
    with open(os.path.join(SKIN_PKG, rel), encoding="utf-8") as f:
        return f.read()


def build_merged_css() -> str:
    skin = read_skin_file("css.txt")
    th1_append = open(os.path.join(HERE, "twowiki-fossil-th1-append.css"), encoding="utf-8").read()
    v6_path = os.path.join(HERE, "one-line-menu-ticket-tags-01a", "twowiki-fossil-skin-v6.css")
    if not os.path.isfile(v6_path):
        print("missing style layer: " + v6_path, file=sys.stderr)
        sys.exit(1)
    v6_full = open(v6_path, encoding="utf-8").read()
    v7_path = os.path.join(HERE, "one-line-menu-ticket-tags-02a", "css.txt")
    use_v7 = os.path.isfile(v7_path)
    if use_v7:
        design = open(v7_path, encoding="utf-8").read()
        # Ticket markdown sortable hooks still live at end of v6 until folded into 02a/css.txt.
        mark = "/* --- Sortable markdown tables"
        p = v6_full.find(mark)
        sort_tail = ("\n\n" + v6_full[p:]) if p >= 0 else ""
        merged_mid = design + sort_tail
    else:
        merged_mid = v6_full
    return skin + "\n\n" + th1_append + "\n\n" + merged_mid


def emit_style_only() -> None:
    css = build_merged_css()
    sys.stdout.write("BEGIN;\n")
    sys.stdout.write(
        "INSERT OR REPLACE INTO config(name,value,mtime) VALUES('default-skin','custom',julianday('now'));\n"
    )
    sys.stdout.write(
        "INSERT OR REPLACE INTO config(name,value,mtime) VALUES(%s,%s,julianday('now'));\n"
        % (esc("css"), esc(css))
    )
    sys.stdout.write("COMMIT;\n")


def emit_full_skin() -> None:
    css = build_merged_css()
    header = read_skin_file("header.txt")
    details = read_skin_file("details.txt")
    js = read_skin_file("js.txt")
    tkt = open(os.path.join(HERE, "ticket-viewpage.th1"), encoding="utf-8").read()
    tkt_edit = open(os.path.join(HERE, "ticket-editpage.th1"), encoding="utf-8").read()
    footer = open(os.path.join(HERE, "footer.th1"), encoding="utf-8").read()
    csp = (
        "default-src 'self' data:; "
        "script-src 'self' 'nonce-$nonce' https://cdn.jsdelivr.net 'wasm-unsafe-eval' blob:; "
        "style-src 'self' 'unsafe-inline'; "
        "img-src * data:; "
        "connect-src 'self' https://cdn.jsdelivr.net data: blob:; "
        "worker-src blob: https://cdn.jsdelivr.net 'wasm-unsafe-eval';"
    )
    sys.stdout.write("BEGIN;\n")
    sys.stdout.write(
        "INSERT OR REPLACE INTO config(name,value,mtime) VALUES('default-skin','custom',julianday('now'));\n"
    )
    for name, val in (
        ("css", css),
        ("header", header),
        ("details", details),
        ("js", js),
        ("ticket-viewpage", tkt),
        ("ticket-editpage", tkt_edit),
        ("footer", footer),
        ("default-csp", csp),
    ):
        sys.stdout.write(
            "INSERT OR REPLACE INTO config(name,value,mtime) VALUES(%s,%s,julianday('now'));\n"
            % (esc(name), esc(val))
        )
    sys.stdout.write("COMMIT;\n")


def main() -> None:
    p = argparse.ArgumentParser(
        description="Emit SQL for Fossil config. Default: style-only (css + default-skin)."
    )
    p.add_argument(
        "--full-skin",
        action="store_true",
        help="Also emit header, details, js, ticket TH1, footer, default-csp (core site config).",
    )
    p.add_argument(
        "--confirm-full",
        action="store_true",
        help="Required with --full-skin so full deploys are never accidental.",
    )
    args = p.parse_args()

    if args.full_skin:
        if not args.confirm_full:
            print(
                "apply_twowiki_skin.py: refusing --full-skin without --confirm-full "
                "(prevents overwriting footer/tickets/CSP by mistake). "
                "For colors/CSS only, run with no arguments.",
                file=sys.stderr,
            )
            sys.exit(2)
        emit_full_skin()
        return

    if args.confirm_full:
        print("apply_twowiki_skin.py: --confirm-full only valid with --full-skin", file=sys.stderr)
        sys.exit(2)

    emit_style_only()


if __name__ == "__main__":
    main()

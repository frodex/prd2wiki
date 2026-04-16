#!/usr/bin/env python3
"""Emit SQL to apply twoWiki Fossil skin overlay. Pipe to: fossil sql -R /path/to/repo.fossil

Layout:
  - ``lovable_01a/`` — Fossil skin package (css.txt, header.txt, details.txt, js.txt): chrome + palette.
  - ``twowiki-fossil-th1-append.css`` — structural CSS (float resets, ticket column, Mermaid overflow).
  - ``one-line-menu-ticket-tags-01a/twowiki-fossil-skin-v6.css`` — design layer (appended last). See ``SKIN-LAYERING.md``.
  - ``footer.th1`` — twoWiki Mermaid/ELK, ticket URL redirect, Setup/skin footer links (not lovable's minimal footer.txt).
  - ``mainmenu.txt`` — Fossil ``config.mainmenu`` (Home, Timeline, Tickets, …); see Fossil help ``mainmenu``.
  - ``ticket-viewpage.th1`` / ``ticket-editpage.th1`` — ticket reader/editor + sortable tables / task lists.
"""
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
# Style-only skin drop-in (Lovable export); edit files here — ``twowiki-fossil-skin-v3.css`` is a duplicate snapshot.
SKIN_PKG = os.path.join(HERE, "lovable_01a")

def esc(s: str) -> str:
    return "'" + s.replace("'", "''") + "'"


def read_skin_file(rel: str) -> str:
    with open(os.path.join(SKIN_PKG, rel), encoding="utf-8") as f:
        return f.read()


def main() -> None:
    skin = read_skin_file("css.txt")
    th1_append = open(os.path.join(HERE, "twowiki-fossil-th1-append.css"), encoding="utf-8").read()
    v6_path = os.path.join(HERE, "one-line-menu-ticket-tags-01a", "twowiki-fossil-skin-v6.css")
    if not os.path.isfile(v6_path):
        print("missing style layer: " + v6_path, file=sys.stderr)
        sys.exit(1)
    v6_style = open(v6_path, encoding="utf-8").read()
    # Merge order: see SKIN-LAYERING.md — v6 last so palette / doc chrome win over stock lovable.
    css = skin + "\n\n" + th1_append + "\n\n" + v6_style
    header = read_skin_file("header.txt")
    details = read_skin_file("details.txt")
    js = read_skin_file("js.txt")
    tkt = open(os.path.join(HERE, "ticket-viewpage.th1"), encoding="utf-8").read()
    tkt_edit = open(os.path.join(HERE, "ticket-editpage.th1"), encoding="utf-8").read()
    footer = open(os.path.join(HERE, "footer.th1"), encoding="utf-8").read()
    mainmenu = open(os.path.join(HERE, "mainmenu.txt"), encoding="utf-8").read().strip() + "\n"
    # $nonce in CSP is replaced by Fossil at runtime (see style.c style_csp).
    # ELK (elkjs) uses WebAssembly; without 'wasm-unsafe-eval' browsers block it and Mermaid falls back
    # or fails — stock Fossil skins often omit default-csp, so this only bites the custom skin.
    # ELK/elkjs: wasm needs 'wasm-unsafe-eval'; workers may be blob: or jsdelivr — include both on script-src + worker-src.
    csp = (
        "default-src 'self' data:; "
        "script-src 'self' 'nonce-$nonce' https://cdn.jsdelivr.net 'wasm-unsafe-eval' blob:; "
        "style-src 'self' 'unsafe-inline'; "
        "img-src * data:; "
        "connect-src 'self' https://cdn.jsdelivr.net data: blob:; "
        "worker-src blob: https://cdn.jsdelivr.net 'wasm-unsafe-eval';"
    )
    sys.stdout.write("BEGIN;\n")
    # Repository `default-skin`: must not name a *built-in* (e.g. plain_gray) or that
    # skin wins over CONFIG (css/header/footer). The literal `custom` is not a built-in
    # label; Fossil falls through to CONFIG skin when no higher-priority override applies.
    # Per-browser `skin=` cookie / URL still outrank this (see /skins?skin=custom).
    sys.stdout.write(
        "INSERT OR REPLACE INTO config(name,value,mtime) VALUES('default-skin','custom',julianday('now'));\n"
    )
    for name, val in (
        ("css", css),
        ("header", header),
        ("details", details),
        ("js", js),
        ("mainmenu", mainmenu),
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


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Emit SQL to apply twoWiki Fossil skin overlay. Pipe to: fossil sql -R /path/to/repo.fossil"""
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))


def esc(s: str) -> str:
    return "'" + s.replace("'", "''") + "'"


def main() -> None:
    css = open(os.path.join(HERE, "twowiki-fossil.css"), encoding="utf-8").read()
    tkt = open(os.path.join(HERE, "ticket-viewpage.th1"), encoding="utf-8").read()
    tkt_edit = open(os.path.join(HERE, "ticket-editpage.th1"), encoding="utf-8").read()
    footer = open(os.path.join(HERE, "footer.th1"), encoding="utf-8").read()
    # $nonce in CSP is replaced by Fossil at runtime (see style.c style_csp).
    csp = (
        "default-src 'self' data:; "
        "script-src 'self' 'nonce-$nonce' https://cdn.jsdelivr.net; "
        "style-src 'self' 'unsafe-inline'; "
        "img-src * data:; "
        "connect-src 'self' https://cdn.jsdelivr.net data: blob:; "
        "worker-src blob:;"
    )
    sys.stdout.write("BEGIN;\n")
    for name, val in (
        ("css", css),
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

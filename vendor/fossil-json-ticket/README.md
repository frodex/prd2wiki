# Fossil JSON ticket API (twoWiki)

This directory holds the **local copy** of changes that add **`/json/ticket/list`**, **`get`**, **`history`**, and **`save`** to a Fossil build with `--json`, plus the small **`tkt.c` exports** needed for `json_ticket.c` to compile.

## Upstream baseline

Captured from the checkout on **twowiki** (`/tmp/fossil-src`):

- **Version:** 2.29 **[cca8cc15f2331578c8667460696d6a042894c7b771c4303a7c1c3c11f247a80e]** (2026-04-13)

Re-applying on a different Fossil revision may require manual conflict resolution.

## Contents

| Path | Purpose |
|------|---------|
| `src/json_ticket.c` | New file: JSON handlers for ticket list/get/history/save. |
| `patches/fossil-json-ticket-from-cca8cc15.patch` | Unified diff for `src/tkt.c`, `src/json.c`, `tools/makemake.tcl`, `src/main.mk`, and Windows makefiles (json_ticket.c wired into the build). |

The patch **does not** add the new `.c` file as a blob; you must copy `src/json_ticket.c` into the Fossil source tree yourself (see below).

## How to apply (Unix)

From the root of a Fossil source tree matching the baseline above:

```sh
cp /path/to/prd2wiki/vendor/fossil-json-ticket/src/json_ticket.c src/json_ticket.c
patch -p0 < /path/to/prd2wiki/vendor/fossil-json-ticket/patches/fossil-json-ticket-from-cca8cc15.patch
```

If `patch` complains about already-applied hunks, use a fresh checkout or merge by hand.

Then configure and build with JSON enabled:

```sh
./configure --json
make -j"$(nproc)"
```

Install the binary as needed (e.g. `/usr/local/bin/fossil-json`).

## HTTP notes

- **Read** JSON endpoints work without extra headers in many setups.
- **Write** paths (including **`/json/ticket/save`**) require a matching **`Referer`** for the server origin (Fossil SQLite authorizer); see Fossil `db.c` / project docs.
- Authenticated JSON calls use **`/json/login`** (e.g. `name` + `p` query parameters) and pass **`authToken`** with a top-level **`payload`** object for POST bodies, per Fossil’s JSON conventions.
- **`GET /json/ticket/history?uuid=…`** (read-only, needs `t` cap): returns a JSON **array** of change rows (`mtime` local string, `login`, `username`, `mimetype`, `icomment`; ascending time). Empty array if no `ticketchng` rows. **404** if uuid prefix matches no ticket.
- **J-card order:** Fossil’s manifest parser requires **`J` lines in ascending field-name order** (see `manifest.c`). The **`json_ticket_save`** handler must emit J-cards in **`aField[]` schema order** (append `+field` keys first, then normal keys), matching **`submit_ticketCmd`** in `tkt.c`. Emitting J lines in JSON object key order can produce a valid-looking artifact that **fails cross-linking**, leaving **`ticket`** rows missing while blobs still grow.

## Provenance

Changes were developed and tested on **twowiki** (`192.168.20.155`); this folder is the **prd2wiki workspace mirror** for version control and review, not a second upstream.

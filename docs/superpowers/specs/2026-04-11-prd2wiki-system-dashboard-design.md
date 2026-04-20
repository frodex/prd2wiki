# DESIGN: prd2wiki System Dashboard

**Date:** 2026-04-11
**Status:** Design — not yet implemented
**Reference:** Pippi Dashboard (v0.1.18 at pippi.pippiai.com) for design patterns

## Purpose

A status and control panel for the wiki showing all services, their health, and system resource usage. Similar to Pippi's dashboard but tailored to the prd2wiki stack.

## Services to Monitor

Canonical **dashboard labels** (same names as the top-row cards and nav dots) map to processes like this:

| Dashboard label | Process / binary | Port/Socket | What to check |
| ----------------- | ---------------- | ----------- | ------------- |
| **WIKI** | `prd2wiki` (HTTP) | `:8082` | HTTP `/health`, page count, goroutines |
| **WIKI** *(sidecar)* | `prd2wiki-mcp` | unix socket | **Unix dial probe** + optional **nav subdot** — see **MCP monitoring** |
| **Database** | `pippi-librarian` | `:19095` + unix socket | HTTP `/health`; **Vector:** LanceDB status, table row counts, vectors; **Keystore:** Badger path size, KV-backed metadata |
| **Embedder** | `tei` (TEI) | `:8088` | HTTP `/health`, model loaded, batch queue |
| **Vault** | *(planned)* | — | Placeholder — **Configured / n/a** until wired |
| **GPU** | `llama-server` (when used) | `:8081` | HTTP health, GPU utilization |

*(Binary paths stay as in deploy — e.g. `/tmp/prd2wiki`, `/srv/pippi-librarian/bin/pippi-librarian` — labels above are UI-only.)*

## System Metrics

| Metric           | Source                             | Display                    |
| ---------------- | ---------------------------------- | -------------------------- |
| CPU % (average)  | `/proc/stat`                       | Gauge + per-core breakdown |
| CPU % (per core) | `/proc/stat`                       | 14 individual gauges       |
| Memory           | `/proc/meminfo`                    | Used / Total MB            |
| Disk             | `syscall.Statfs`                   | Used / Total GB            |
| GPU %            | Intel GPU sysfs or `intel_gpu_top` | Gauge (0-100%)             |
| GPU Freq         | sysfs                              | Current / Max MHz          |
| Goroutines       | `runtime.NumGoroutine()`           | Count                      |
| Uptime           | process start time                 | Duration                   |

## Data Metrics

| Metric | Maps to dashboard | Source | Display |
| ------ | ----------------- | ------ | ------- |
| Total wiki pages | **WIKI** | prd2wiki index | Count per project |
| Memory / entity records | **Database · Vector** | pippi-librarian memory\_search / tables | Count |
| LanceDB tables / vectors | **Database · Vector** | pippi-librarian status | Table names + row counts |
| Badger / KV footprint | **Database · Keystore** | directory size on Badger path | MB on disk |
| LanceDB files on disk | **Database · Vector** | directory size | MB on disk |
| Vector dimensions | **Embedder** | embedder config | Number |
| Embedding model | **Embedder** | embedder config | Name + params |

## Service Health Cards

Each **dashboard label** gets a card using the same titles as the **Proposed Layout** row (WIKI, Database, Embedder, Vault, GPU). Example **Database** card (Vector + Keystore in one tile):

```
┌────────────────────────────────┐
│ Database (pippi-librarian)     │
│ ┌─────────┐                    │
│ │ Running │  Connected         │
│ └─────────┘                    │
│ Vector:  126 rows · 7.5 MB     │
│ Keystore: 2.1 MB Badger        │
└────────────────────────────────┘
```

Example **WIKI** card:

```
┌─────────────────────────────┐
│ WIKI                         │
│ ┌─────────┐                  │
│ │ Running │  Connected       │
│ └─────────┘                  │
│ Pages: 126 · Projects: 5     │
│ MCP: Listening (unix)        │  ← when prd2wiki-mcp configured
└─────────────────────────────┘
```

Example **Database** card with MCP line (pippi-librarian exposes MCP over **unix** in addition to HTTP — see repo `cmd/pippi-librarian/main.go`):

```
┌────────────────────────────────┐
│ Database (pippi-librarian)     │
│ …                              │
│ HTTP: OK · MCP (unix): OK      │
└────────────────────────────┘
```

Status states:

* **Running** (green) — process alive, health endpoint responds

* **Degraded** (yellow) — process alive, health endpoint slow or partial (e.g., LanceDB still hydrating)

* **Stopped** (red) — process not found or health fails

* **Configured** (blue) — configured but not started

* **Connected** (green badge) — successfully communicating with other services

## Design Reference: Pippi Dashboard

Pippi's dashboard (v0.1.18) shows:

* Service cards: pippi-core, Vault, LLM, GPU — each with Running/Connected/Configured status

* System gauges: CPU %, Memory, Disk, DB Size, Goroutines, Uptime, GPU %, GPU Freq

* Per-core CPU breakdown (14 individual gauges)

* 2-minute sparkline charts for CPU, Memory, Goroutines, GPU

We should reuse this layout pattern. Pippi's dashboard code is in the Pippi monorepo — check if components can be used directly.

## Implementation Options

### Option A: Static page with JS polling

Add a `/dashboard` route to prd2wiki's web handler. JavaScript polls health endpoints every 5 seconds. No new dependencies.

```
GET /dashboard
  → renders dashboard.html template
  → JS fetches (labels = dashboard cards):
    GET /health                         → WIKI
    GET http://127.0.0.1:19095/health   → Database (Vector + Keystore)
    GET http://127.0.0.1:8088/health      → Embedder
    GET /api/projects/default/stats     → WIKI stats
    (+ GPU / Vault probes when configured)
```

### Option B: Server-side with SSE

Dashboard page with Server-Sent Events for real-time updates. Server goroutine polls services and pushes updates.

### Option C: Reuse Pippi's dashboard code

Pippi's dashboard is a Go web handler + HTML/CSS/JS. If the layout and polling logic can be extracted, we avoid rebuilding from scratch.

## Proposed Layout

Top service row uses **canonical labels** (WIKI · Database · Embedder · Vault · GPU), not internal binary names. **Database** is one card with **Vector** + **Keystore** called out on the second line.

```
┌────────────────────────────────────────────────────────────────────────────┐
│ System Dashboard (prd2wiki)                                         v0.x.x │
├────────────────────────────────────────────────────────────────────────────┤
│                                                                            │
│  ┌──────────┐ ┌──────────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐     │
│  │  WIKI    │ │  Database    │ │ Embedder │ │  Vault   │ │   GPU    │     │
│  │ Running  │ │ Running      │ │ Running  │ │Configured│ │ Connected│     │
│  │Connected │ │Vector·Keystore│ │Connected │ │   n/a    │ │          │     │
│  └──────────┘ └──────────────┘ └──────────┘ └──────────┘ └──────────┘     │
│                                                              │
│  ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ │
│  │CPU │ │MEM │ │DISK│ │DB  │ │GRTN│ │UPTM│ │GPU │ │FREQ│ │
│  │20% │ │11G │ │35G │ │10M │ │ 19 │ │2d  │ │ 0% │ │2GHz│ │
│  └────┘ └────┘ └────┘ └────┘ └────┘ └────┘ └────┘ └────┘ │
│                                                              │
│  ┌──────┐┌──────┐┌──────┐┌──────┐┌──────┐┌──────┐┌──────┐ │
│  │ 98%  ││ 45%  ││ 12%  ││  8%  ││  6%  ││  9%  ││  7%  │ │
│  │core0 ││core1 ││core2 ││core3 ││core4 ││core5 ││core6 │ │
│  └──────┘└──────┘└──────┘└──────┘└──────┘└──────┘└──────┘ │
│  ┌──────┐┌──────┐┌──────┐┌──────┐┌──────┐┌──────┐┌──────┐ │
│  │  5%  ││ 11%  ││  3%  ││  7%  ││ 14%  ││  4%  ││  6%  │ │
│  │core7 ││core8 ││core9 ││cor10 ││cor11 ││cor12 ││cor13 │ │
│  └──────┘└──────┘└──────┘└──────┘└──────┘└──────┘└──────┘ │
│                                                              │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐           │
│  │CPU % (2 min)│ │MEM MB(2 min)│ │GPU % (2 min)│           │
│  │ ▁▂▃▅▃▂▁▂▃▂ │ │ ▇▇▇▇▇▇▇▇▇▇ │ │ ▁▁▁▁█▁▁▁▁▁ │           │
│  └─────────────┘ └─────────────┘ └─────────────┘           │
│                                                              │
│  Data: 126 pages · 5 projects · 3856 vectors                │
│  Disk: BadgerDB 2.1 MB · LanceDB 7.5 MB · Total 9.6 MB    │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

## Data Summary Row

At the bottom, aggregate stats across all services (wording aligned with **WIKI** / **Database · Vector & Keystore** / **Embedder**):

* Total pages across all projects (**WIKI**)

* Total disk: git + SQLite index + **Vector** (Lance) + **Keystore** (Badger) paths

* Total vectors (**Database · Vector**)

* Embedder model name and dimension count (**Embedder**)

## API Endpoints Needed

| Endpoint | Feeds dashboard label | Process | Returns |
| -------- | ---------------------- | ------- | ------- |
| `GET /health` | **WIKI** | prd2wiki | `{"status":"ok"}` |
| `GET /api/dashboard/stats` | **WIKI** (+ aggregates) | prd2wiki (new) | page counts, project list, disk usage |
| `GET /health` | **Database** | pippi-librarian | health + record counts (split Vector vs Keystore in JSON if available) |
| `GET /health` | **Embedder** | TEI | `{"status":"ok"}` or model info |
| `GET /api/dashboard/system` | host metrics (all cards) | prd2wiki (new) | CPU, memory, disk, GPU from `/proc` / `Statfs` |

*(**Vault** and **GPU** use the same system or process-specific probes once defined.)*

## Design decisions (owner input integrated)

* **Where it lives:** A dedicated **`/dashboard`** page *and* a **Dashboard** link in the **top nav** (same chrome as the rest of the wiki). **Compact health indicators** in the nav: small **round dots** — **green** = healthy, **gray** = unknown / degraded / not checked, **red** = error — for **core** services (same health signals as the full dashboard, summarized).
* **Reuse Pippi:** Prefer **reusing Pippi’s dashboard code as-is** where licensing and structure allow — audit the Pippi monorepo and copy or vendor the handler + static assets rather than re-spec’ing from memory.
* **Polling cadence (draft):** **~5 s** for lightweight **health** probes, **~30 s** for heavier **stats** (disk, counts, sparkline buffers) — **must be confirmed** against Pippi’s actual intervals (see research plan below).
* **Auth:** Dashboard and nav indicators are **public** (no login) on the same host/LAN assumptions as today’s wiki.
* **Sparklines & per-service usage:** Include **short rolling sparklines** (same spirit as Pippi’s ~2 min traces). Where possible, show **per-service memory (RSS)** and **disk** (data dir / DB paths) — exact attribution TBD per process.
* **Card / column naming (canonical):**

| Label | Meaning |
| ----- | ------- |
| **WIKI** | prd2wiki HTTP server + index/git |
| **Database** | Split into two sub-areas: **Vector** (Lance / vector index + embeddings footprint) and **Keystore** (Badger / KV metadata — keys, vocab, federation state, etc., as exposed by pippi-librarian) |
| **Embedder** | TEI / llama embedding path |
| **Vault** | Reserved placeholder — adopt **soon**; show **Configured / n/a** until wired |
| **GPU** | GPU embedder or Intel GPU metrics as today |

**Services to Monitor**, **Data Metrics**, **Proposed Layout**, **API Endpoints**, and **Implementation Option A** all use these labels consistently; only footnotes mention binary paths.

* **Pippi lineage (approved):** Pippi’s dashboard code **may be adapted**, but the **core** (layout rhythm, status semantics, polling + sparkline behavior, visual language) should stay **as close to upstream Pippi as practical**. When we must change something, prefer **thin wrappers** and **config-driven differences** over rewriting internals — so we can track a **baseline tag** (e.g. v0.1.18) and optionally merge fixes from Pippi later.

## Reusable dashboard core (cross-project)

Goal: a **generic, adaptable block** usable in prd2wiki **and** other repos (Pippi services, sidecars, future stacks), with **clear edges** and **configuration** instead of copy-paste forks.

### Layers and boundaries

| Layer | Owns | Stays generic | Project-specific (inject / config) |
| ----- | ---- | --------------- | ----------------------------------- |
| **UI shell** | Card grid, nav dots, gauges, sparkline strips, CSS variables | HTML structure, class names, JS polling loop shape | Brand string, logo link, which cards are visible, theme overrides via CSS vars |
| **Data contract** | Versioned JSON the UI consumes (`services[]`, `host`, `series`) | Field names for status, timestamps, numeric series | Extra optional fields per service (e.g. `vector_bytes`, `keystore_bytes`) ignored if absent |
| **Aggregator** | Single HTTP handler(s) that return the contract | Aggregation pattern (parallel fetch, timeouts, classify errors → red/gray) | List of **probes** and URLs loaded from **config** |
| **Probes** | `HealthProbe` / `StatsProbe` (or one interface) per logical service | HTTP GET with timeout; **unix dial** for MCP sockets; map → normalized status | URL bases, mTLS, auth headers, unix paths, response parsers (librarian vs TEI shapes) |
| **Host metrics** | CPU, RAM, disk, optional GPU from OS | Interface `HostSampler` + Linux implementation reading `/proc`, `Statfs`, sysfs | Paths, permission model (same-host only), number of CPU cores |
| **Service catalog** | Declarative list: id, **dashboard label** (WIKI, Database, …), poll tier (fast/slow), probe bindings | Schema validation | YAML/JSON embedded in each app or file on disk |

**Edges (interfaces):** keep **Go interfaces** at the **aggregator ↔ probes** and **aggregator ↔ host** boundaries so tests use fakes and another project swaps YAML + probe implementations only. On the **front end**, one **fetch** to `GET /api/dashboard/snapshot` (name TBD) keeps the embedded JS free of prd2wiki URLs.

### Configuration (generic but adaptable)

* **Service definitions:** id, display name, group (for nav dots), `health_url`, optional `stats_url`, optional **unix MCP** paths, optional `rss_pid` or `data_paths[]` for disk, poll interval override.
* **Feature flags:** enable Vault column, GPU row; **MCP** uses per-service `mcp_sidecar` / `unix_mcp_socket` (see **MCP monitoring**) rather than a global flag.
* **Safety:** bind aggregator to **loopback** or explicit allowlist in config; no open proxy by default.

### Packaging options (later)

1. **Phase 1 — inside prd2wiki:** package as `internal/dashboard/` (or `internal/opsdashboard/`) with interfaces above; Pippi assets vendored next to it.
2. **Phase 2 — extract:** move stable core to a small **shared module** (module path TBD) consumed by prd2wiki and, if desired, Pippi, with **only** config + probe binaries differing.

### Alignment with Pippi

When lifting code from Pippi, **mark** each file as *vendor vs adapted*; keep diffs **minimal**. Prefer **exporting** constants (poll ms, sparkline length) to a **single config struct** shared with the JSON/YAML loader so behavioral parity is one place to compare.

## Pippi as-built (reference host, 2026-04-11)

**Source:** SSH host **`pippi`** (`192.168.23.171`), monorepo **`/opt/pippi`**. This section records what was actually in the tree — not inferred from the public web (which sits behind Cloudflare Access).

### Routing and templates

| Item | Location / behavior |
| ---- | ------------------- |
| **Dashboard URL** | **`GET /`** (home) — **not** `/dashboard`. Handler: `handleIndex` in `internal/ui/web/web.go`. Template: `internal/ui/web/templates/dashboard.gohtml`. |
| **SSR data** | `dashboardData`: task count, provider count, **`VaultOK`** via `vaultClient.Health(ctx)`, **`LLMConfigured`** via Vault secret reads for enabled providers. |
| **Static assets** | `internal/ui/web/static/*`; templates use Pico CSS + Chart.js (CDN) on pages that need charts. |

### Service cards (top row on home dashboard)

Four **fixed** `<article>` blocks in `dashboard.gohtml` — not yet data-driven:

| Header | How status is determined |
| ------ | ------------------------- |
| **pippi-core** | Static badge **Running** (no runtime probe). |
| **Vault** | Server-rendered: **Connected** vs **Unavailable** from `VaultOK`. |
| **LLM** | Server-rendered: **Configured** vs **Not configured** from Vault API keys for providers. |
| **GPU** | Initial **Checking…**; live-updated from SSE (see below). |

### Host metrics (CPU, memory, disk, DB size, goroutines, GPU)

| Item | Location |
| ---- | -------- |
| **Collector** | `internal/platform/sysinfo/sysinfo.go` — type **`Metrics`** (JSON-tagged), **`Collector.Collect()`** reads `/proc/stat` (per-core CPU %), `/proc/meminfo`, disk usage for **`/`**, SQLite **DB file size** from configured path, **Intel iGPU** via sysfs (`GPUMetrics`), **`runtime.NumGoroutine()`**, process uptime. |
| **Publish loop** | `cmd/pippi-core/main.go` — **`time.NewTicker(2 * time.Second)`** → `Collect()` → JSON marshal → **`broker.Publish("system:metrics", data)`**. |
| **Transport to browser** | `internal/platform/httpapi/server.go` — **`GET /api/v1/events`** exposes the SSE **broker**; clients receive events by name. |

**Important:** Pippi’s **live host strip** is driven at **2 s**, not 5 s. Any **5 s / 30 s** cadence in our design is an **additional** layer for prd2wiki (e.g. slow aggregates), not a match to Pippi’s core ticker.

### Browser wiring

* Dashboard loads **`sse.js`**; **`pippiSSE.on("system:metrics", …)`** in `dashboard.gohtml` updates DOM ids **`ds-cpu`**, **`ds-mem`**, **`ds-disk`**, **`ds-gr`**, **`ds-up`**, and GPU badge / freq / busy when `m.gpu.present`.
* **Sparklines** labeled **“2 min”** live on **`tasks_list.gohtml`**, using the **same** `system:metrics` stream to push points into canvas history — not a separate HTTP poll interval.

### Edges to exploit (maps to reusable core)

1. **`sysinfo.Collector` + `Metrics` JSON** — Strong candidate to **vendor verbatim** as the **host / CPU / GPU “pre-packaged adapter”** (parameterize disk mount and DB path).
2. **SSE event name + payload shape** — **`system:metrics`** is a stable **contract** between collector and UI; prd2wiki could mirror the event name or map into a unified **`/api/dashboard/snapshot`** for non-SSE clients.
3. **Service cards** — Today **hardcoded**; replacing with **`range` over a config slice** is the main **template edge** for WIKI / Database / Embedder / Vault / GPU without fork drift in the **sys-summary** row.
4. **Vault / LLM** — **Tightly coupled** to `vault.Client` + settings store — treat as **custom adapters**; prd2wiki would use **HTTP probes** to librarian / wiki index instead.

## Draft service catalog (YAML sketch — prd2wiki)

Illustrative only: field names should align with the **aggregator config** section above; bind addresses come from each environment.

```yaml
# e.g. embed in binary, or load from a single file beside prd2wiki config
dashboard:
  listen: "127.0.0.1:8083"           # aggregator UI + JSON; TBD vs same port as wiki
  host_metrics:
    interval: 2s                     # parity with Pippi system:metrics ticker
    disk_mount: "/"
    # db_file_for_size: optional; prd2wiki may use sqlite path for “DB bytes” row
  slow_aggregates_interval: 30s      # librarian table counts, wiki page totals, etc.
  nav_dots_order: [WIKI, Database, Embedder, GPU]
  services:
    - id: wiki
      label: WIKI
      health_url: "http://127.0.0.1:8082/health"
      poll: fast
      mcp_sidecar:                    # optional; omit entire block to hide
        unix_socket: "/run/prd2wiki-mcp.sock"
        label: "WIKI · MCP"
        nav_subdot: true              # second dot under WIKI in nav strip
    - id: librarian
      label: Database
      health_url: "http://127.0.0.1:19095/health"
      poll: slow
      unix_mcp_socket: "/var/run/pippi-librarian.sock"   # optional; default in librarian config when `socket` unset
      stats:
        vector_data_path: "/srv/pippi-librarian/data/lance"   # example
        keystore_path: "/srv/pippi-librarian/data/badger"     # example
    - id: embedder
      label: Embedder
      health_url: "http://127.0.0.1:8088/health"
      poll: slow
    - id: vault
      label: Vault
      mode: placeholder              # until HTTP or sidecar exists
      display: "n/a"
    - id: gpu
      label: GPU
      mode: host_sysfs               # Intel path: reuse sysinfo GPU fields
      optional_llm_health: "http://127.0.0.1:8081/health"
  feature_flags:
    vault_column: false
```

**Notes:** **Fast** vs **slow** maps to two poll loops (2 s host + cheap HTTP; 30 s heavy stats). **Vault** stays a stub row until a real probe exists. **GPU** combines **host adapter** data with an optional **HTTP probe** to `llama-server` when that stack is used. **MCP** unix paths are **optional**; when set, the aggregator runs **unix dial probes** on the fast loop (see **MCP monitoring** below).

## MCP monitoring (approved)

The stack exposes **two** MCP surfaces that matter for ops visibility — different roles, same probe primitive:

| Surface | Process | Typical transport | Dashboard placement |
| ------- | ------- | ----------------- | --------------------- |
| **WIKI · MCP** | `prd2wiki-mcp` | Unix socket (sidecar to wiki HTTP) | Sub-line on **WIKI** card; optional **second nav dot** under **WIKI** when `nav_subdot: true`. |
| **Database · MCP** | `pippi-librarian` | Unix socket **and** HTTP `/sse`, `/tools/call` on loopback | Single **Database** card: show **HTTP** + **MCP (unix)** as two checks — not a separate top-level label. |

**Probe semantics (v1):** implement a **`UnixDialProbe`** — `net.DialTimeout("unix", path, ~300ms)` (exact timeout in config). Success ⇒ **Listening** / **OK**; `ENOENT` or refuse ⇒ **Down** with short reason. **No full MCP JSON-RPC handshake** in v1 unless we later need “degraded but listening”; dial-only matches “is the sidecar accepting connections?”

**Why a nav subdot for WIKI · MCP only:** the wiki operator often cares whether **agents can attach** via MCP independently of whether **pages render**. A nested dot keeps the **primary** dot row at five conceptual services (WIKI, Database, Embedder, GPU) without inventing a sixth top-level name. **Database** MCP status stays **inside** the Database card so the dot row does not double-count librarian.

**Security:** probes run **only from the same host** as the dashboard (loopback aggregator). Socket paths must **not** be exposed to remote callers via the dashboard API — treat them like file paths in config.

**Aggregator interface:** extend the generic **probe** list with `kind: unix` + `path` so the same code path can serve wiki sidecar and librarian socket without one-off handlers.

## Pippi file → layer map (what to vendor vs rewrite)

Use this when copying from **`/opt/pippi`** (confirm **git tag** on the host before freezing a vendor snapshot).

| Path | Layer | prd2wiki action |
| ---- | ----- | ---------------- |
| `internal/ui/web/templates/dashboard.gohtml` | UI shell | **Adapt:** replace fixed four cards with `range` over config; keep Pico + structure. |
| `internal/ui/web/templates/tasks_list.gohtml` | UI shell (sparklines) | **Optional vendor:** if tasks view is out of scope for v1, defer; sparkline JS pattern is reusable. |
| `internal/ui/web/web.go` | Aggregator + SSR | **Rewrite thin:** swap Vault/LLM SSR for HTTP probes + wiki/librarian-derived fields. |
| `internal/platform/sysinfo/sysinfo.go` | Host adapter | **Vendor** (parameterize paths / mount / DB file). |
| `cmd/pippi-core/main.go` | Loop + broker | **Pattern copy:** ticker → `Collect` → publish; wire to prd2wiki **SSE or snapshot** endpoint. |
| `internal/platform/httpapi/server.go` | HTTP / SSE | **Adapt routes:** keep **`/api/v1/events`**-style SSE or add **`GET /api/dashboard/snapshot`** for polling clients. |
| `internal/ui/web/static/*` (e.g. `sse.js`) | Transport | **Vendor** minimal diff if event names stay stable. |
| Vault / settings / task store | Custom probes | **Do not port** — replace with **HTTP** health to prd2wiki + librarian as in YAML above. |

## Nav dots and process model (working assumptions)

| Topic | Assumption for v1 |
| ----- | ------------------- |
| **Nav dots** | One dot per **core** service: **WIKI**, **Database**, **Embedder**, **GPU**. **Vault** appears as a **card** (or column) when `feature_flags.vault_column` is true, not necessarily as a dot unless product wants it. |
| **WIKI sidecar (MCP)** | When `services.wiki.mcp_sidecar` is set, show card line + optional **`nav_subdot`** (see **MCP monitoring**). Omit the block entirely if no wiki MCP in this deploy. |
| **RSS / per-PID memory** | **Defer:** prd2wiki and pippi-librarian run as **single OS processes** each; TEI and llama are **separate** processes. v1 focuses on **host** CPU/RAM/disk + **HTTP health**; optional later: map `pids[]` in config for `ps`-style RSS rows. |
| **Disk** | Show **host mount** (e.g. `/`) plus **configured paths** for Lance / Badger **directory size** under **Database** metrics — matches the **Data Metrics** table earlier in this doc. |

## Remaining research (before implementation)

* **Freeze vendor baseline:** record **exact git commit** from `pippi` next to any copied files (tag v0.1.18 or current `HEAD`).
* **Single vs split port:** **Resolved** — **embedded** in prd2wiki (see **Implementation plan**); loopback guard on **`/ops/*`** and **`/api/ops/*`** only.
* **SSE vs snapshot-only:** if some clients cannot use SSE, specify **`/api/ops/snapshot`** polling interval and how it relates to the 2 s / 30 s split (see **Implementation plan**).
* **Unix MCP dial vs permissions:** pippi-librarian creates the socket at **`0660`** (`cmd/pippi-librarian/main.go`). Ensure the dashboard process **runs as a user/group** that can connect, or document that probes run as **root** on the ops host only.

## Implementation plan (2026-04-11)

**Executable plan (task list, code pointers, commits):** prd2wiki repo path **`docs/superpowers/plans/2026-04-11-system-dashboard.md`** (Go module `github.com/frodex/prd2wiki`).

**Wiki mirror (same content):** page **`48c86ff`** — *PLAN: prd2wiki System Dashboard Implementation*.

**Locked decisions (supersede earlier “TBD” lines in this page where they conflict):**

* **Port / embedding:** Dashboard is **embedded in the prd2wiki HTTP server** (same mux as wiki + API), not a separate loopback-only `:8083` process — loopback restriction applies to **`/ops/*`** and **`/api/ops/*`** only.
* **API paths:** **`GET /api/ops/snapshot`** (JSON v1 contract in plan), **`GET /api/ops/events`** (SSE), **`GET /ops/`** (HTML). Older draft names like `/api/dashboard/snapshot` are **not** used.
* **pippi-librarian:** Add **`GET /health`** on the existing MCP HTTP mux (prerequisite for Database HTTP probe); unix MCP dial remains as designed.
* **prd2wiki-mcp:** Still **stdio-only** in tree; **WIKI · MCP** unix row stays **off** until optional unix transport is implemented — see plan **Task 11**.

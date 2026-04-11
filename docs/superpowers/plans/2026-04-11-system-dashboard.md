> **Cross-references:** Canonical design wiki page **`f7f7bd1`** (*DESIGN: prd2wiki System Dashboard*). Repository copy: `docs/superpowers/plans/2026-04-11-system-dashboard.md` (commit with code changes; this page can be updated via API or ingest).

---

# prd2wiki System Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship an **operations dashboard** inside **prd2wiki**: JSON snapshot API, optional SSE stream for live host metrics, loopback-only access, HTTP + unix-dial probes for WIKI / Database / Embedder / Vault placeholder / GPU, wired to the architecture in wiki page **`f7f7bd1`** (“DESIGN: prd2wiki System Dashboard”).

**Architecture:** New Go package **`internal/dashboard`** holds config types, probes (`HealthProbe` HTTP, `UnixDialProbe`), a **Linux host sampler** (vendored from Pippi `sysinfo` patterns — CPU/mem/disk/gpu from `/proc`, `Statfs`, sysfs), an **aggregator** that runs fast (≈2 s) and slow (≈30 s) loops, and **HTTP handlers** for `GET /api/ops/snapshot` and `GET /api/ops/events` (SSE). The **root mux** in `internal/app/app.go` registers routes under `/api/ops/` and serves a minimal **HTML dashboard** at **`GET /ops/`** (or `/ops/dashboard`) using `embed` for templates/static. **pippi-librarian** gains a trivial **`GET /health`** on its existing MCP HTTP mux so the Database card can use the same HTTP probe primitive as WIKI/Embedder without MCP auth.

**Tech stack:** Go 1.25+, `net/http`, `html/template`, `gopkg.in/yaml.v3`, Linux-only host metrics (build tags). No new third-party deps unless tests need them.

**Canonical spec:** Wiki **`f7f7bd1`** — labels (WIKI, Database, Embedder, Vault, GPU), MCP unix-dial semantics, YAML sketch, JSON contract goals.

---

## File map (create / modify)

| Path | Role |
| ---- | ---- |
| `internal/dashboard/config.go` | YAML-unmarshal structs for `dashboard:` section; defaults; validation. |
| `internal/dashboard/snapshot.go` | Versioned JSON DTOs returned by `/api/ops/snapshot`. |
| `internal/dashboard/probe_http.go` | HTTP GET with timeout → `ProbeResult`. |
| `internal/dashboard/probe_unix.go` | `DialTimeout("unix", path)` → `ProbeResult`. |
| `internal/dashboard/host_linux.go` | `HostSampler` implementation (Linux). |
| `internal/dashboard/host_stub.go` | `//go:build !linux` empty or minimal host metrics. |
| `internal/dashboard/aggregator.go` | Combines probes + host + wiki index stats + optional dir sizes. |
| `internal/dashboard/handler.go` | `http.Handler` for snapshot, SSE, and HTML page. |
| `internal/dashboard/loopback.go` | Middleware: allow only loopback (and optional Unix domain socket peer). |
| `internal/dashboard/templates/dashboard.html` | Pico-style layout; cards from config. |
| `internal/dashboard/static/dashboard.js` | Fetch snapshot + optional `EventSource` for SSE. |
| `internal/app/app.go` | Load dashboard config; register routes; pass `Searcher`/DB for page counts. |
| `config/prd2wiki.yaml` | Example `dashboard:` block (commented). |
| `cmd/pippi-librarian/main.go` (sibling repo) | Register **`GET /health`** on librarian `mux`. |

---

## Snapshot JSON contract (v1)

Implement exactly these shapes in `snapshot.go` so the UI and external tools stay stable:

```go
package dashboard

import "time"

// Snapshot is the response body for GET /api/ops/snapshot (version 1).
type Snapshot struct {
	Version     int       `json:"version"` // always 1
	GeneratedAt time.Time `json:"generated_at"`
	Host        HostSnap  `json:"host"`
	Services    []ServiceSnap `json:"services"`
}

type HostSnap struct {
	CPUAvgPct   float64   `json:"cpu_avg_pct"`
	MemUsedMB   int64     `json:"mem_used_mb"`
	MemTotalMB  int64     `json:"mem_total_mb"`
	DiskUsedGB  float64   `json:"disk_used_gb"`
	DiskTotalGB float64   `json:"disk_total_gb"`
	Goroutines  int       `json:"goroutines"`
	UptimeSec   int64     `json:"uptime_sec"`
	GPU         *GPUSnap  `json:"gpu,omitempty"`
}

type GPUSnap struct {
	Present bool    `json:"present"`
	BusyPct float64 `json:"busy_pct"`
	FreqMHz int64   `json:"freq_mhz"`
}

type ServiceSnap struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Overall string `json:"overall"` // ok | degraded | down | unknown | n/a

	HTTP    *ProbeResult `json:"http,omitempty"`
	UnixMCP *ProbeResult `json:"unix_mcp,omitempty"`
	WikiMCP *ProbeResult `json:"wiki_mcp,omitempty"`

	Detail map[string]string `json:"detail,omitempty"`
}

type ProbeResult struct {
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}
```

**SSE events (v1):** `GET /api/ops/events` sends `Content-Type: text/event-stream`. One event name: **`system:metrics`**. Payload: JSON object matching **`HostSnap`** (same as Pippi’s coupling: browser updates gauges from host-only updates). Service cards update from periodic **`snapshot`** fetch or a second event **`services`** — **YAGNI for v1:** use **SSE for host only**; **reload snapshot** every 30 s from JS or trigger snapshot refetch after SSE connect. Simplest v1: **single SSE event** `tick` with full `Snapshot` JSON every 2 s (slightly heavier but one source of truth). **Pick one in Task 7 (HTTP handlers)** and document in handler comment; recommended: **full `Snapshot` every 2 s on SSE** to avoid drift between host strip and cards.

---

### Task 1: Snapshot DTOs (`snapshot.go`)

**Files:**
- Create: `/srv/prd2wiki/internal/dashboard/snapshot.go`

- [ ] **Step 1:** Paste the **Snapshot JSON contract (v1)** types from the top of this plan into `snapshot.go` (same package `dashboard`).

- [ ] **Step 2: Commit**

```bash
cd /srv/prd2wiki && git add internal/dashboard/snapshot.go && git commit -m "feat(dashboard): snapshot JSON types"
```

---

### Task 2: Config types and validation

**Files:**
- Create: `/srv/prd2wiki/internal/dashboard/config.go`
- Test: `/srv/prd2wiki/internal/dashboard/config_test.go`

- [ ] **Step 1: Add structs matching the wiki YAML sketch**

```go
package dashboard

import (
	"fmt"
	"time"
)

type Config struct {
	Enabled bool `yaml:"enabled"`

	HostInterval       time.Duration `yaml:"host_interval"`
	SlowInterval       time.Duration `yaml:"slow_interval"`
	DiskMount          string        `yaml:"disk_mount"`
	DBFileForSize      string        `yaml:"db_file_for_size"`

	Services []ServiceCfg `yaml:"services"`
}

type ServiceCfg struct {
	ID       string `yaml:"id"`
	Label    string `yaml:"label"`
	HealthURL string `yaml:"health_url"`
	Poll     string `yaml:"poll"` // fast | slow

	UnixMCPSocket string `yaml:"unix_mcp_socket"`
	MCPSidecar    *struct {
		UnixSocket string `yaml:"unix_socket"`
		Label      string `yaml:"label"`
		NavSubdot  bool   `yaml:"nav_subdot"`
	} `yaml:"mcp_sidecar"`

	VectorDataPath string `yaml:"vector_data_path"`
	KeystorePath   string `yaml:"keystore_path"`
}

// ApplyDefaults sets HostInterval (2s), SlowInterval (30s), DiskMount (/) when zero/empty.
// Call after yaml.Unmarshal into Config (including when embedded in app.Config).
func ApplyDefaults(c *Config) {
	if c == nil {
		return
	}
	if c.HostInterval == 0 {
		c.HostInterval = 2 * time.Second
	}
	if c.SlowInterval == 0 {
		c.SlowInterval = 30 * time.Second
	}
	if c.DiskMount == "" {
		c.DiskMount = "/"
	}
}

func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	for _, s := range c.Services {
		if s.ID == "" {
			return fmt.Errorf("dashboard service missing id")
		}
		if s.Label == "" {
			return fmt.Errorf("dashboard service %q missing label", s.ID)
		}
	}
	return nil
}
```

Note: if embedding `Config` inside `app.Config`, use a pointer `*dashboard.Config` with `yaml:"dashboard"` and default `enabled: false` until ready.

- [ ] **Step 2: Write test for defaults**

```go
func TestConfigDefaults(t *testing.T) {
	var c Config
	raw := []byte("enabled: true\nservices:\n  - id: wiki\n    label: WIKI\n")
	if err := yaml.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	ApplyDefaults(&c)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.HostInterval != 2*time.Second {
		t.Fatalf("host interval: %v", c.HostInterval)
	}
}
```

Run: `cd /srv/prd2wiki && go test ./internal/dashboard/ -run TestConfigDefaults -v`

Expected: PASS (complete **Task 1** first so `ProbeResult` exists for probes package.)

- [ ] **Step 3: Commit**

```bash
cd /srv/prd2wiki && git add internal/dashboard/config.go internal/dashboard/config_test.go && git commit -m "feat(dashboard): config structs and validation"
```

---

### Task 3: HTTP and unix probes + tests

**Files:**
- Create: `/srv/prd2wiki/internal/dashboard/probe_http.go`
- Create: `/srv/prd2wiki/internal/dashboard/probe_unix.go`
- Create: `/srv/prd2wiki/internal/dashboard/probe_test.go`

- [ ] **Step 1: Implement probes**

```go
package dashboard

import (
	"context"
	"net"
	"net/http"
	"time"
)

func ProbeHTTP(ctx context.Context, client *http.Client, url string) ProbeResult {
	if client == nil {
		client = http.DefaultClient
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ProbeResult{OK: false, Error: err.Error()}
	}
	res, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return ProbeResult{OK: false, LatencyMs: latency, Error: err.Error()}
	}
	defer res.Body.Close()
	ok := res.StatusCode >= 200 && res.StatusCode < 300
	pr := ProbeResult{OK: ok, LatencyMs: latency}
	if !ok {
		pr.Error = http.StatusText(res.StatusCode)
	}
	return pr
}

func ProbeUnix(ctx context.Context, path string, timeout time.Duration) ProbeResult {
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	c, err := d.DialContext(ctx, "unix", path)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return ProbeResult{OK: false, LatencyMs: latency, Error: err.Error()}
	}
	_ = c.Close()
	return ProbeResult{OK: true, LatencyMs: latency}
}
```

- [ ] **Step 2: Test with `httptest.Server` and temp unix socket**

Use `net.Listen("unix", socketPath)` in test (skip if OS does not support — build tag or `t.Skip`).

Run: `go test ./internal/dashboard/ -v -count=1`

Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/dashboard/probe_*.go internal/dashboard/probe_test.go && git commit -m "feat(dashboard): HTTP and unix dial probes"
```

---

### Task 4: Librarian prerequisite — `GET /health`

**Files:**
- Modify: `/srv/pippi-librarian/cmd/pippi-librarian/main.go` (after `mux := http.NewServeMux()` and **before** `/sse` if you want unauthenticated health; place **before** encrypted routes — health must not require MCP auth.)

- [ ] **Step 1: Register handler**

```go
mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
})
```

- [ ] **Step 2: Build**

Run: `cd /srv/pippi-librarian && go build -o /dev/null ./cmd/pippi-librarian/`

Expected: success.

- [ ] **Step 3: Commit (pippi-librarian repo)**

```bash
cd /srv/pippi-librarian && git add cmd/pippi-librarian/main.go && git commit -m "feat(http): add GET /health on MCP mux for ops probes"
```

---

### Task 5: Host metrics (Linux)

**Files:**
- Create: `/srv/prd2wiki/internal/dashboard/host_linux.go` (`//go:build linux`)
- Create: `/srv/prd2wiki/internal/dashboard/host_stub.go` (`//go:build !linux`)

- [ ] **Step 1: Define interface in `host.go` (no build tag)**

```go
package dashboard

import "context"

type HostSampler interface {
	Sample(ctx context.Context) (HostSnap, error)
}
```

- [ ] **Step 2: Implement Linux sampler** by porting logic from Pippi `internal/platform/sysinfo/sysinfo.go` (vendor copy, same package `dashboard`, **do not** import Pippi module — not a dependency). Include: CPU avg, mem, disk for `DiskMount`, goroutines, uptime, optional Intel GPU sysfs if files exist. Parameterize `DiskMount` and optional SQLite path for “DB file size” if design requires — can be **phase 2**; v1 may omit DB file size row to reduce scope.

- [ ] **Step 3: Unit test** with mocked `/proc` — optional; minimal compile test: `go test ./internal/dashboard/ -tags=linux`.

Run: `go test ./internal/dashboard/ -v`

- [ ] **Step 4: Commit**

```bash
git add internal/dashboard/host*.go && git commit -m "feat(dashboard): Linux host metrics sampler"
```

---

### Task 6: Aggregator

**Files:**
- Create: `/srv/prd2wiki/internal/dashboard/aggregator.go`

- [ ] **Step 1: Define `Aggregator` struct** holding `Config`, `*http.Client`, `HostSampler`, and **`WikiStats`** interface:

```go
type WikiStats interface {
	TotalPages(ctx context.Context, project string) (int, error)
}
```

- [ ] **Step 2: Implement `Collect(ctx) (Snapshot, error)`** that:
  - Sets `Version: 1`, `GeneratedAt: time.Now().UTC()`
  - Fills `Host` from `HostSampler`
  - For each `ServiceCfg`, maps to `ServiceSnap`:
    - If `HealthURL` non-empty: `ProbeHTTP` with 2 s timeout for `fast`, 10 s for `slow` (constants in file)
    - If `UnixMCPSocket` non-empty: `ProbeUnix` with 300 ms timeout
    - If `MCPSidecar` non-empty: fill `WikiMCP` from unix probe
    - Derive `Overall`: if HTTP required and fails → `down`; if unix fails but HTTP ok → `degraded` for Database; if Vault has no URL → `n/a`
  - `Detail`: e.g. `pages` count for wiki from `WikiStats` using first project from config or `"default"`

- [ ] **Step 3: Optional slow path** (same method or separate `CollectSlow` merged into snapshot): directory sizes for `VectorDataPath` / `KeystorePath` using `filepath.WalkDir` summing `FileInfo.Size()` — cap max walk time or skip on error.

- [ ] **Step 4: Commit**

```bash
git add internal/dashboard/aggregator.go && git commit -m "feat(dashboard): aggregator for snapshot"
```

---

### Task 7: HTTP handlers + loopback guard

**Files:**
- Create: `/srv/prd2wiki/internal/dashboard/loopback.go`
- Create: `/srv/prd2wiki/internal/dashboard/handler.go`

- [ ] **Step 1: Loopback middleware**

```go
package dashboard

import (
	"net"
	"net/http"
	"strings"
)

func LoopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addr := r.RemoteAddr
		if strings.HasPrefix(addr, "@") {
			next.ServeHTTP(w, r)
			return
		}
		host, _, err := net.SplitHostPort(addr)
		if err != nil || (host != "127.0.0.1" && host != "::1") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

Unix-domain upstreams sometimes use `@` — allow through for local socket tests; tighten policy later if needed.

- [ ] **Step 2: Register handlers**
  - `GET /api/ops/snapshot` → JSON `Snapshot`
  - `GET /api/ops/events` → SSE (recommended: full snapshot JSON every `HostInterval`, `event: tick` or single unnamed data lines)
  - `GET /ops/` → HTML dashboard

- [ ] **Step 3: Test with `httptest`**

Assert 403 from non-loopback (simulate with `RemoteAddr` injection via `httptest` client — may need custom `http.Server` or request rewriting).

Run: `go test ./internal/dashboard/ -v`

- [ ] **Step 4: Commit**

```bash
git add internal/dashboard/handler.go internal/dashboard/loopback.go && git commit -m "feat(dashboard): HTTP handlers and loopback middleware"
```

---

### Task 8: Wire into `app.New`

**Files:**
- Modify: `/srv/prd2wiki/internal/app/app.go`
- Modify: `/srv/prd2wiki/internal/app/app.go` `Config` struct — add `Dashboard *dashboard.Config \`yaml:"dashboard"\``

- [ ] **Step 1:** If `cfg.Dashboard != nil && cfg.Dashboard.Enabled`, construct `HostSampler`, `Aggregator`, wrap `dashboard.Handler`, apply `LoopbackOnly` to `/api/ops/` and `/ops/`.

- [ ] **Step 2:** Implement `WikiStats` on a small type in `app` package using `index.Searcher` or SQL: `SELECT COUNT(*) FROM pages` (match existing schema — inspect `internal/index/sqlite.go` for table name).

- [ ] **Step 3:** Register on **root mux** **before** heavy middleware if health-like behavior desired for `/ops` — keep logging middleware consistent with `/api/`.

Run: `go build -o /tmp/prd2wiki ./cmd/prd2wiki/`

Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go && git commit -m "feat(dashboard): wire ops routes into app mux"
```

---

### Task 9: Templates and static assets

**Files:**
- Create: `/srv/prd2wiki/internal/dashboard/embed.go` with `//go:embed templates/* static/*`
- Create: `templates/dashboard.html`, `static/dashboard.js`

- [ ] **Step 1:** HTML: card grid for each `ServiceSnap`, host strip for `HostSnap`, optional nav subdots for `WikiMCP` when `nav_subdot` in config (pass from handler).

- [ ] **Step 2:** JS: `fetch('/api/ops/snapshot')` on load; `new EventSource('/api/ops/events')` to refresh host numbers or full snapshot.

- [ ] **Step 3: Commit**

```bash
git add internal/dashboard/embed.go internal/dashboard/templates internal/dashboard/static && git commit -m "feat(dashboard): embedded dashboard UI"
```

---

### Task 10: Example config and docs pointer

**Files:**
- Modify: `/srv/prd2wiki/config/prd2wiki.yaml`

- [ ] **Step 1:** Add commented `dashboard:` example with `health_url` pointing to `http://127.0.0.1:8080/health` (match default `server.addr`), librarian `http://127.0.0.1:19095/health` **after Task 4 (librarian `/health`) deployed**, embedder URL, `unix_mcp_socket: /var/run/pippi-librarian.sock`.

- [ ] **Step 2: Commit**

```bash
git add config/prd2wiki.yaml && git commit -m "docs(config): example dashboard section"
```

---

### Task 11: prd2wiki-mcp note (no unix socket yet)

**Behavior:** Current `cmd/prd2wiki-mcp` uses **stdio** only (`ServeStdio`). **Unix-dial `WikiMCP` row stays hidden** until a future change adds optional unix (or SSE) transport to `prd2wiki-mcp`. Do **not** block dashboard v1 on this; config omits `mcp_sidecar` until then.

Record as **follow-up** in `f7f7bd1` or this plan’s “Out of scope” — no code task now.

---

## Self-review (spec coverage)

| f7f7bd1 topic | Task |
| ------------- | ---- |
| Canonical labels WIKI / Database / Embedder / Vault / GPU | `ServiceCfg.Label` + template |
| HTTP + unix MCP probes | Tasks 3, 6 |
| 2 s host / 30 s slow | `HostInterval`, `SlowInterval`, aggregator (Task 6) |
| Loopback-only safety | Task 7 |
| JSON snapshot for UI | Tasks 1–2, 6–7 |
| MCP: WIKI sidecar + Database unix | Task 6; WIKI gated on future MCP transport |
| Pippi lineage (vendor sysinfo) | Task 5 |
| Librarian HTTP health | Task 4 |

**Placeholder scan:** No TBD in executable steps; optional items explicitly deferred (wiki MCP unix, DB file size row).

---

## Out of scope (follow-up releases)

- Full MCP JSON-RPC health handshake.
- Vault real probe.
- Non-Linux host metrics (stub returns `unknown`).
- **prd2wiki-mcp** unix listener — add when product prioritizes agent attach visibility.

---

## Execution handoff

**Plan complete and saved to** `docs/superpowers/plans/2026-04-11-system-dashboard.md`.

**1. Subagent-driven (recommended)** — dispatch a fresh subagent per task, review between tasks.

**2. Inline execution** — run tasks in order in one session with checkpoints after Tasks 3, 6, and 8.

**Which approach?**

# Constraints: Phase 3a.7 — Wire libclient + syncToLibrarian

**Scope:** Connect prd2wiki to pippi-librarian over unix socket. When a page is edited, call `memory_store` and write the returned `record_id` to `.link` line 2.
**Repo:** `/srv/prd2wiki/` — branch `impl/phase-3a7`
**Cross-repo contract:** `/srv/prd2wiki/docs/constraints-prd2wiki-pippi.md`
**Librarian MCP schema:** `/srv/pippi-librarian/schema.d/wiki.yaml`

## What to build

### 1. `internal/libclient/client.go` — remote librarian client

HTTP client that talks to pippi-librarian over unix socket (or TCP loopback). Calls MCP `/tools/call` endpoint.

**Methods needed:**

```go
type Client struct { ... }

func New(socketPath string, apiKey string) *Client

// MemoryStore creates or updates a wiki page in the librarian.
// Returns the new head record_id (mem_ ID).
func (c *Client) MemoryStore(ctx context.Context, namespace, pageUUID, content string, metadata map[string]any) (string, error)
```

**MCP call format** (what the librarian expects):
```json
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "memory_store",
    "arguments": {
      "namespace": "wiki:{project-uuid}",
      "page_uuid": "550e8400-...",
      "content": "# Page content...",
      "metadata": {
        "source_repo": "proj_8f04b8f5.git",
        "source_commit": "abc123",
        "page_title": "My Page",
        "page_type": "reference",
        "page_status": "draft",
        "page_tags": "tag1,tag2",
        "author": "mcp-agent"
      }
    }
  }
}
```

**Response:** JSON with `record_id` (string), `version` (int), `created` (bool).

**Transport:** The librarian listens on a unix socket at the path in config `librarian.socket`. If empty or not configured, libclient is nil and syncToLibrarian remains a no-op.

**Auth:** The librarian uses ticket-based auth over the socket. For the initial implementation, the socket's peer credentials (UID/GID) provide auth — the librarian trusts local callers. If a Bearer token is needed, pass it in the config.

### 2. Wire into `internal/librarian/librarian.go`

The `syncToLibrarian` stub already exists (added in Pre-flight A item 1). It currently no-ops when `PageUUID == ""`. Wire it to the libclient:

```go
func (l *Librarian) syncToLibrarian(req SubmitRequest, path, commitHash string) {
    if l.libClient == nil || req.PageUUID == "" { return }
    go func() {
        ext := map[string]any{
            "source_repo":   "proj_" + req.ProjectUUID[:8] + ".git",
            "source_branch": req.Branch,
            "source_commit": commitHash,
            "page_title":    req.Frontmatter.Title,
            "page_type":     req.Frontmatter.Type,
            "page_status":   req.Frontmatter.Status,
            "page_tags":     strings.Join(req.Frontmatter.Tags, ","),
            "author":        req.Author,
        }
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        newHeadID, err := l.libClient.MemoryStore(ctx, "wiki:"+req.ProjectUUID, req.PageUUID, string(req.Body), ext)
        if err != nil {
            slog.Warn("librarian sync failed", "page", req.PageUUID, "err", err)
            return
        }
        // Write new head ID to .link line 2
        // (tree.IndexHolder has a method for this, or write directly)
    }()
}
```

### 3. Wire into `internal/app/app.go`

- Read `librarian.socket` from config
- If non-empty, create `libclient.New(socketPath, "")`
- Pass to `librarian.New(...)` or set on the Librarian struct
- If socket not configured or empty, libclient is nil — wiki works without librarian

### 4. .link line 2 write-back

After `MemoryStore` returns the `record_id`, update `.link` line 2 for that page's UUID. The tree `IndexHolder` should have access to the tree root. Write line 2 without changing lines 1 and 3.

Mutex per `page_uuid` to prevent concurrent edits from tearing the `.link` file.

## What NOT to do

- **Don't implement `memory_search`, `memory_get`, or `memory_delete` calls** — only `memory_store` for now
- **Don't block page saves on librarian** — `syncToLibrarian` runs in a goroutine. Git save always succeeds first.
- **Don't add retry queue yet** — that's a follow-up. Log the error and move on.
- **Don't change pippi-librarian code** — libclient is prd2wiki only

## Config

```yaml
librarian:
  socket: "/var/run/pippi-librarian.sock"  # empty = disabled
```

## Build/test

```bash
cd /srv/prd2wiki
go build ./...
go test ./...
```

## Gate

- `libclient.New()` connects to unix socket
- `MemoryStore()` calls librarian and returns `record_id`
- `syncToLibrarian` calls libclient when `PageUUID != ""`
- `.link` line 2 updated after successful store
- Wiki still works when `librarian.socket` is empty (no-op)
- `go test ./...` passes

## Stop conditions

- If the librarian socket protocol isn't standard HTTP/JSON-RPC → **stop, ask** (check how the MCP server listens)
- If auth over the socket requires something other than peer credentials → **stop, ask**
- If `.link` line 2 write causes race conditions with the tree scanner → **stop, ask**

# Phase 4: MCP Sidecar — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build an MCP server sidecar that exposes the wiki to AI agents via the Model Context Protocol. Agents can read pages, search, propose changes, challenge content, and ingest sources — all through standard MCP resources, tools, and prompts.

**Architecture:** Separate Go binary (`cmd/prd2wiki-mcp/`) that speaks MCP (JSON-RPC over stdio) to agents and REST to the wiki core API. Runs alongside the wiki server, not compiled into it. Can be updated independently.

**Tech Stack:** Go, `github.com/modelcontextprotocol/go-sdk` (MCP Go SDK), `net/http` client for wiki API calls

**Spec Reference:** `/srv/prd2wiki/docs/superpowers/specs/2026-04-07-prd2wiki-design-04.md` (Section 11)

---

## File Structure

```
cmd/
└── prd2wiki-mcp/
    └── main.go                    # MCP server binary entrypoint
internal/
└── mcp/
    ├── server.go                  # MCP server setup, tool/resource registration
    ├── server_test.go
    ├── client.go                  # HTTP client for wiki core API
    ├── client_test.go
    ├── resources.go               # MCP resource handlers (read-only context)
    ├── tools.go                   # MCP tool handlers (agent-invocable actions)
    └── prompts.go                 # MCP prompt templates
```

---

### Task 1: Wiki API Client

**Files:**
- Create: `internal/mcp/client.go`
- Create: `internal/mcp/client_test.go`

HTTP client that wraps calls to the wiki core REST API. All MCP handlers use this client.

```go
package mcp

type WikiClient struct {
    baseURL string
    http    *http.Client
}

func NewWikiClient(baseURL string) *WikiClient

// Page operations
func (c *WikiClient) GetPage(project, id, branch string) (*PageResponse, error)
func (c *WikiClient) CreatePage(project string, req CreatePageRequest) (*CreatePageResponse, error)
func (c *WikiClient) ListPages(project string, filters map[string]string) ([]PageResult, error)
func (c *WikiClient) DeletePage(project, id, branch string) error

// Search
func (c *WikiClient) Search(project string, params map[string]string) ([]PageResult, error)

// References
func (c *WikiClient) GetReferences(project, id string, depth int) (*ReferencesResponse, error)
```

Response types mirror the API's JSON responses. Test with httptest mock server.

- [ ] Implement and commit

### Task 2: MCP Server + Resources

**Files:**
- Create: `internal/mcp/server.go`
- Create: `internal/mcp/resources.go`

Set up the MCP server using the Go SDK. Register resources:

- `wiki://project-a/PRD-042` — read a specific page (content + frontmatter)
- `wiki://project-a/index` — page catalog for a project
- `wiki://project-a/contested` — pages with active challenges

Resources are read-only context that agents pull. Use the `server.Resource` registration from the MCP Go SDK.

The resource URI template: `wiki://{project}/{page_id}` for pages, `wiki://{project}/index` for catalog.

- [ ] Implement and commit

### Task 3: MCP Tools

**Files:**
- Create: `internal/mcp/tools.go`
- Create: `internal/mcp/tools_test.go`

Register MCP tools (agent-invocable actions):

1. `wiki_search` — full-text + metadata search
   - Input: `{"project": "...", "query": "...", "type": "...", "status": "...", "tag": "..."}`
   - Calls WikiClient.Search

2. `wiki_read` — read a specific page
   - Input: `{"project": "...", "id": "...", "branch": "..."}`
   - Calls WikiClient.GetPage

3. `wiki_propose` — create/update page on a draft branch
   - Input: `{"project": "...", "id": "...", "title": "...", "type": "...", "body": "...", "tags": [...], "intent": "verbatim|conform|integrate"}`
   - Calls WikiClient.CreatePage

4. `wiki_challenge` — raise a challenge against a page
   - Input: `{"project": "...", "target_id": "...", "evidence": "...", "reason": "..."}`
   - Creates a challenge page on a challenge/* branch via WikiClient.CreatePage

5. `wiki_ingest` — submit external source material
   - Input: `{"project": "...", "url": "...", "title": "...", "kind": "...", "content": "..."}`
   - Creates a source page on ingest/* branch

6. `wiki_lint` — check a page's provenance chain
   - Input: `{"project": "...", "id": "..."}`
   - Calls WikiClient.GetReferences, checks for stale/contested statuses

7. `wiki_status` — check status of a page or project
   - Input: `{"project": "...", "id": "..."}`
   - Returns page metadata or project summary

Each tool has a JSON Schema input definition and returns structured text output.

- [ ] Implement and commit

### Task 4: MCP Prompts

**Files:**
- Create: `internal/mcp/prompts.go`

Register MCP prompts (reusable workflow templates):

1. `review_page` — structured review template
   - Arguments: project, page_id
   - Returns a prompt that guides the agent through reviewing a page's content, provenance, and consistency

2. `ingest_source` — guided source registration
   - Arguments: project, url, title
   - Returns a prompt that guides source evaluation and registration

- [ ] Implement and commit

### Task 5: MCP Binary + Integration

**Files:**
- Create: `cmd/prd2wiki-mcp/main.go`
- Modify: `Makefile` — add mcp build target

The MCP binary:
1. Reads wiki API base URL from flag or environment (`PRDWIKI_API_URL`, default `http://localhost:8080`)
2. Creates a WikiClient
3. Creates the MCP server with all resources, tools, and prompts registered
4. Starts the MCP server on stdio (standard MCP transport for CLI tools)

```go
func main() {
    apiURL := os.Getenv("PRDWIKI_API_URL")
    if apiURL == "" {
        apiURL = "http://localhost:8080"
    }
    
    client := mcp.NewWikiClient(apiURL)
    srv := mcp.NewServer(client)
    
    // Run on stdio
    srv.ServeStdio()
}
```

Add to Makefile:
```makefile
build-mcp:
	go build -o bin/prd2wiki-mcp ./cmd/prd2wiki-mcp
```

- [ ] Implement and commit

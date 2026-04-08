package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newMCPServer creates an MCPServer backed by a mock wiki API.
func newMCPServer(t *testing.T, mux *http.ServeMux) (*MCPServer, func()) {
	t.Helper()
	srv := httptest.NewServer(mux)
	client := NewWikiClient(srv.URL)
	return NewServer(client), srv.Close
}

// callTool invokes a tool handler directly and returns the decoded result.
func callTool(t *testing.T, s *MCPServer, name string, args interface{}) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	rt, ok := s.tools[name]
	if !ok {
		t.Fatalf("tool %q not registered", name)
	}
	result, err := rt.Handler(raw)
	if err != nil {
		t.Fatalf("%s returned error: %v", name, err)
	}
	out, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	return out
}

// callToolExpectError invokes a tool handler and expects an error.
func callToolExpectError(t *testing.T, s *MCPServer, name string, args interface{}) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	rt, ok := s.tools[name]
	if !ok {
		t.Fatalf("tool %q not registered", name)
	}
	_, err = rt.Handler(raw)
	if err == nil {
		t.Fatalf("expected %s to return error, got nil", name)
	}
}

// rpcCall sends a JSON-RPC request through the server's Serve loop and returns
// the response.
func rpcCall(t *testing.T, s *MCPServer, method string, params interface{}) JSONRPCResponse {
	t.Helper()
	reqObj := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
	}
	if params != nil {
		raw, _ := json.Marshal(params)
		reqObj["params"] = json.RawMessage(raw)
	}
	reqLine, _ := json.Marshal(reqObj)

	var out bytes.Buffer
	s.Serve(bytes.NewReader(append(reqLine, '\n')), &out)

	var resp JSONRPCResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v (raw: %s)", err, out.String())
	}
	return resp
}

// ---------------------------------------------------------------------------
// Server protocol tests
// ---------------------------------------------------------------------------

func TestInitialize(t *testing.T) {
	mux := http.NewServeMux()
	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	resp := rpcCall(t, s, "initialize", nil)
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", resp.Result)
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion: got %v", result["protocolVersion"])
	}
	info, _ := result["serverInfo"].(map[string]interface{})
	if info["name"] != "prd2wiki" {
		t.Errorf("serverInfo.name: got %v", info["name"])
	}
}

func TestToolsList(t *testing.T) {
	mux := http.NewServeMux()
	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	resp := rpcCall(t, s, "tools/list", nil)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error)
	}

	result, _ := resp.Result.(map[string]interface{})
	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatalf("expected tools array, got %T", result["tools"])
	}
	if len(tools) != 7 {
		t.Errorf("expected 7 tools, got %d", len(tools))
	}

	// Check that known tool names are present.
	names := make(map[string]bool)
	for _, tool := range tools {
		tm, _ := tool.(map[string]interface{})
		names[fmt.Sprint(tm["name"])] = true
	}
	for _, want := range []string{"wiki_search", "wiki_read", "wiki_propose", "wiki_challenge", "wiki_ingest", "wiki_lint", "wiki_status"} {
		if !names[want] {
			t.Errorf("missing tool %q", want)
		}
	}
}

func TestResourcesList(t *testing.T) {
	mux := http.NewServeMux()
	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	resp := rpcCall(t, s, "resources/list", nil)
	if resp.Error != nil {
		t.Fatalf("resources/list error: %v", resp.Error)
	}

	result, _ := resp.Result.(map[string]interface{})
	templates, ok := result["resourceTemplates"].([]interface{})
	if !ok {
		t.Fatalf("expected resourceTemplates array, got %T", result["resourceTemplates"])
	}
	if len(templates) != 2 {
		t.Errorf("expected 2 resource templates, got %d", len(templates))
	}
}

func TestMethodNotFound(t *testing.T) {
	mux := http.NewServeMux()
	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	resp := rpcCall(t, s, "bogus/method", nil)
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code: got %d, want -32601", resp.Error.Code)
	}
}

// ---------------------------------------------------------------------------
// Tool handler tests
// ---------------------------------------------------------------------------

func TestWikiSearchTool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/myproj/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "requirement" {
			t.Errorf("expected type=requirement, got %q", r.URL.Query().Get("type"))
		}
		json.NewEncoder(w).Encode([]PageResult{
			{ID: "req-001", Title: "First Req", Type: "requirement", Status: "draft"},
		})
	})

	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	out := callTool(t, s, "wiki_search", map[string]string{
		"project": "myproj",
		"type":    "requirement",
	})

	var results []PageResult
	if err := json.Unmarshal(out, &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "req-001" {
		t.Errorf("ID: got %q, want req-001", results[0].ID)
	}
}

func TestWikiSearchToolMissingProject(t *testing.T) {
	mux := http.NewServeMux()
	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	callToolExpectError(t, s, "wiki_search", map[string]string{})
}

func TestWikiReadTool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages/req-001", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(PageResponse{
			ID: "req-001", Title: "Req 1", Type: "requirement", Body: "# Hello",
		})
	})

	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	out := callTool(t, s, "wiki_read", map[string]string{
		"project": "proj",
		"id":      "req-001",
	})

	var page PageResponse
	if err := json.Unmarshal(out, &page); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if page.ID != "req-001" {
		t.Errorf("ID: got %q", page.ID)
	}
	if page.Body != "# Hello" {
		t.Errorf("Body: got %q", page.Body)
	}
}

func TestWikiReadToolMissingFields(t *testing.T) {
	mux := http.NewServeMux()
	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	callToolExpectError(t, s, "wiki_read", map[string]string{"project": "proj"})
}

func TestWikiProposeTool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		var req CreatePageRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Branch != "draft/agent" {
			t.Errorf("branch: got %q, want draft/agent", req.Branch)
		}
		if req.Author != "mcp-agent" {
			t.Errorf("author: got %q, want mcp-agent", req.Author)
		}
		if req.ID != "new-page" {
			t.Errorf("ID: got %q, want new-page", req.ID)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreatePageResponse{ID: req.ID, Status: "draft"})
	})

	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	out := callTool(t, s, "wiki_propose", map[string]interface{}{
		"project": "proj",
		"id":      "new-page",
		"title":   "New Page",
		"type":    "concept",
		"body":    "# Content",
		"tags":    []string{"test"},
	})

	var resp CreatePageResponse
	json.Unmarshal(out, &resp)
	if resp.ID != "new-page" {
		t.Errorf("ID: got %q", resp.ID)
	}
}

func TestWikiChallengeTool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		var req CreatePageRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Branch != "challenge/req-001" {
			t.Errorf("branch: got %q, want challenge/req-001", req.Branch)
		}
		if req.Status != "contested" {
			t.Errorf("status: got %q, want contested", req.Status)
		}
		if req.Type != "challenge" {
			t.Errorf("type: got %q, want challenge", req.Type)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreatePageResponse{ID: req.ID, Status: "contested"})
	})

	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	callTool(t, s, "wiki_challenge", map[string]string{
		"project":   "proj",
		"target_id": "req-001",
		"evidence":  "The requirement contradicts spec section 3.2",
		"reason":    "Inconsistency with architectural decision",
	})
}

func TestWikiIngestTool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		var req CreatePageRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Branch != "ingest/sources" {
			t.Errorf("branch: got %q, want ingest/sources", req.Branch)
		}
		if req.Type != "source" {
			t.Errorf("type: got %q, want source", req.Type)
		}
		if !strings.HasPrefix(req.ID, "src-") {
			t.Errorf("ID should start with src-, got %q", req.ID)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreatePageResponse{ID: req.ID, Status: "draft"})
	})

	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	callTool(t, s, "wiki_ingest", map[string]string{
		"project": "proj",
		"title":   "API Design Doc",
		"url":     "https://example.com/doc",
		"kind":    "design-doc",
		"content": "The API should support REST and GraphQL",
	})
}

func TestWikiLintTool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages/req-001/references", func(w http.ResponseWriter, r *http.Request) {
		root := RefNode{
			Ref:    "req-001",
			Status: "root",
			Children: []RefNode{
				{Ref: "src-001", Status: "valid", Children: []RefNode{}},
				{Ref: "src-002", Status: "stale", Children: []RefNode{}},
				{Ref: "src-003", Status: "contested", Children: []RefNode{}},
			},
		}
		json.NewEncoder(w).Encode(root)
	})

	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	out := callTool(t, s, "wiki_lint", map[string]string{
		"project": "proj",
		"id":      "req-001",
	})

	var result map[string]interface{}
	json.Unmarshal(out, &result)
	if result["ok"] != false {
		t.Error("expected ok=false for lint with issues")
	}
	issues, _ := result["issues"].([]interface{})
	if len(issues) != 2 {
		t.Errorf("expected 2 issues (stale + contested), got %d", len(issues))
	}
}

func TestWikiLintToolClean(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages/req-001/references", func(w http.ResponseWriter, r *http.Request) {
		root := RefNode{
			Ref:    "req-001",
			Status: "root",
			Children: []RefNode{
				{Ref: "src-001", Status: "valid", Children: []RefNode{}},
			},
		}
		json.NewEncoder(w).Encode(root)
	})

	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	out := callTool(t, s, "wiki_lint", map[string]string{
		"project": "proj",
		"id":      "req-001",
	})

	var result map[string]interface{}
	json.Unmarshal(out, &result)
	if result["ok"] != true {
		t.Error("expected ok=true for clean lint")
	}
}

func TestWikiStatusToolPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages/req-001", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(PageResponse{
			ID: "req-001", Title: "Req 1", Type: "requirement", Status: "approved", TrustLevel: 3,
		})
	})

	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	out := callTool(t, s, "wiki_status", map[string]string{
		"project": "proj",
		"id":      "req-001",
	})

	var result map[string]interface{}
	json.Unmarshal(out, &result)
	if result["id"] != "req-001" {
		t.Errorf("id: got %v", result["id"])
	}
	if result["status"] != "approved" {
		t.Errorf("status: got %v", result["status"])
	}
}

func TestWikiStatusToolProject(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]PageResult{
			{ID: "a", Type: "requirement", Status: "draft"},
			{ID: "b", Type: "requirement", Status: "approved"},
			{ID: "c", Type: "concept", Status: "draft"},
		})
	})

	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	out := callTool(t, s, "wiki_status", map[string]string{
		"project": "proj",
	})

	var result map[string]interface{}
	json.Unmarshal(out, &result)
	if result["project"] != "proj" {
		t.Errorf("project: got %v", result["project"])
	}
	total, _ := result["total"].(float64)
	if total != 3 {
		t.Errorf("total: got %v", total)
	}
}

// ---------------------------------------------------------------------------
// Resource handler tests
// ---------------------------------------------------------------------------

func TestResourceReadIndex(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]PageResult{
			{ID: "a", Title: "A"},
			{ID: "b", Title: "B"},
		})
	})

	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	resp := rpcCall(t, s, "resources/read", map[string]string{
		"uri": "wiki://proj/index",
	})
	if resp.Error != nil {
		t.Fatalf("resources/read error: %v", resp.Error)
	}
}

func TestResourceReadPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/pages/req-001", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(PageResponse{ID: "req-001", Title: "Req 1"})
	})

	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	resp := rpcCall(t, s, "resources/read", map[string]string{
		"uri": "wiki://proj/req-001",
	})
	if resp.Error != nil {
		t.Fatalf("resources/read error: %v", resp.Error)
	}
}

func TestResourceReadBadScheme(t *testing.T) {
	mux := http.NewServeMux()
	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	resp := rpcCall(t, s, "resources/read", map[string]string{
		"uri": "http://proj/index",
	})
	if resp.Error == nil {
		t.Fatal("expected error for non-wiki:// scheme")
	}
}

// ---------------------------------------------------------------------------
// Tools/call via JSON-RPC
// ---------------------------------------------------------------------------

func TestToolsCallViaRPC(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/proj/search", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]PageResult{{ID: "x", Title: "X"}})
	})

	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	resp := rpcCall(t, s, "tools/call", map[string]interface{}{
		"name":      "wiki_search",
		"arguments": map[string]string{"project": "proj"},
	})
	if resp.Error != nil {
		t.Fatalf("tools/call error: %v", resp.Error)
	}

	result, _ := resp.Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	if len(content) == 0 {
		t.Fatal("expected content array")
	}
}

func TestToolsCallUnknownTool(t *testing.T) {
	mux := http.NewServeMux()
	s, cleanup := newMCPServer(t, mux)
	defer cleanup()

	resp := rpcCall(t, s, "tools/call", map[string]interface{}{
		"name":      "nonexistent",
		"arguments": map[string]string{},
	})
	if resp.Error == nil {
		t.Fatal("expected error for unknown tool")
	}
}

// ---------------------------------------------------------------------------
// slugify
// ---------------------------------------------------------------------------

func TestSlugify(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"API Design Doc", "api-design-doc"},
		{"Hello World!", "hello-world"},
		{"  spaces  ", "spaces"},
		{"123-test", "123-test"},
	}
	for _, tt := range tests {
		got := slugify(tt.in)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

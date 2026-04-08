package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// MCPServer implements a minimal Model Context Protocol server over JSON-RPC 2.0
// on stdio. It dispatches tool calls and resource reads to registered handlers.
type MCPServer struct {
	client    *WikiClient
	tools     map[string]registeredTool
	resources map[string]ResourceHandler
	prompts   map[string]registeredPrompt
}

// registeredTool pairs a tool definition with its handler function.
type registeredTool struct {
	Def     ToolDef
	Handler ToolHandler
}

// JSONRPCRequest is a JSON-RPC 2.0 request envelope.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response envelope.
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ToolDef describes a tool exposed via MCP.
type ToolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

// ToolHandler processes a tool call and returns a result or error.
type ToolHandler func(params json.RawMessage) (interface{}, error)

// ResourceHandler reads a resource identified by URI and returns content.
type ResourceHandler func(uri string) (interface{}, error)

// ResourceDef describes a resource template exposed via MCP.
type ResourceDef struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType,omitempty"`
}

// NewServer creates an MCPServer backed by the given WikiClient.
func NewServer(client *WikiClient) *MCPServer {
	s := &MCPServer{
		client:    client,
		tools:     make(map[string]registeredTool),
		resources: make(map[string]ResourceHandler),
	}
	s.registerTools()
	s.registerResources()
	s.registerPrompts()
	return s
}

// RegisterTool adds a tool to the server.
func (s *MCPServer) RegisterTool(def ToolDef, handler ToolHandler) {
	s.tools[def.Name] = registeredTool{Def: def, Handler: handler}
}

// RegisterResource adds a resource handler keyed by a URI pattern prefix.
func (s *MCPServer) RegisterResource(pattern string, handler ResourceHandler) {
	s.resources[pattern] = handler
}

// ServeStdio runs the MCP server main loop: reads newline-delimited JSON-RPC
// requests from stdin and writes responses to stdout.
func (s *MCPServer) ServeStdio() {
	s.Serve(os.Stdin, os.Stdout)
}

// Serve runs the main loop reading from r and writing to w. This is the
// testable core of ServeStdio.
func (s *MCPServer) Serve(r io.Reader, w io.Writer) {
	scanner := bufio.NewScanner(r)
	// Allow large messages (16 MB).
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp := JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &RPCError{Code: -32700, Message: "parse error"},
			}
			writeResponse(w, resp)
			continue
		}

		resp := s.dispatch(req)
		writeResponse(w, resp)
	}
}

func (s *MCPServer) dispatch(req JSONRPCRequest) JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	case "resources/list":
		return s.handleResourcesList(req)
	case "resources/read":
		return s.handleResourcesRead(req)
	case "prompts/list":
		return s.handlePromptsList(req)
	case "prompts/get":
		return s.handlePromptsGet(req)
	default:
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
		}
	}
}

func (s *MCPServer) handleInitialize(req JSONRPCRequest) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools":     map[string]interface{}{},
				"resources": map[string]interface{}{},
				"prompts":   map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "prd2wiki",
				"version": "0.1.0",
			},
		},
	}
}

func (s *MCPServer) handleToolsList(req JSONRPCRequest) JSONRPCResponse {
	defs := make([]ToolDef, 0, len(s.tools))
	for _, rt := range s.tools {
		defs = append(defs, rt.Def)
	}
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"tools": defs},
	}
}

func (s *MCPServer) handleToolsCall(req JSONRPCRequest) JSONRPCResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32602, Message: "invalid params: " + err.Error()},
		}
	}

	rt, ok := s.tools[params.Name]
	if !ok {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32602, Message: fmt.Sprintf("unknown tool: %s", params.Name)},
		}
	}

	result, err := rt.Handler(params.Arguments)
	if err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": fmt.Sprintf("error: %s", err.Error())},
				},
				"isError": true,
			},
		}
	}

	// Marshal result to text content.
	text, _ := json.Marshal(result)
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": string(text)},
			},
		},
	}
}

func (s *MCPServer) handleResourcesList(req JSONRPCRequest) JSONRPCResponse {
	templates := []ResourceDef{
		{
			URITemplate: "wiki://{project}/index",
			Name:        "Wiki Page Index",
			Description: "List all pages in a project",
			MimeType:    "application/json",
		},
		{
			URITemplate: "wiki://{project}/{id}",
			Name:        "Wiki Page",
			Description: "Read a specific wiki page",
			MimeType:    "application/json",
		},
	}
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"resourceTemplates": templates},
	}
}

func (s *MCPServer) handleResourcesRead(req JSONRPCRequest) JSONRPCResponse {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32602, Message: "invalid params: " + err.Error()},
		}
	}

	// Parse wiki:// URI
	uri := params.URI
	if !strings.HasPrefix(uri, "wiki://") {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32602, Message: "unsupported URI scheme, expected wiki://"},
		}
	}

	path := strings.TrimPrefix(uri, "wiki://")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32602, Message: "invalid URI: expected wiki://{project}/{id|index}"},
		}
	}

	project := parts[0]
	resource := parts[1]

	var handler ResourceHandler
	if resource == "index" {
		handler = s.resources["index"]
	} else {
		handler = s.resources["page"]
	}

	if handler == nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32602, Message: "no handler for resource"},
		}
	}

	// Pass project and resource info through the URI itself.
	result, err := handler(project + "/" + resource)
	if err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32603, Message: err.Error()},
		}
	}

	text, _ := json.Marshal(result)
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"uri":      params.URI,
					"mimeType": "application/json",
					"text":     string(text),
				},
			},
		},
	}
}

func (s *MCPServer) handlePromptsList(req JSONRPCRequest) JSONRPCResponse {
	defs := make([]PromptDef, 0, len(s.prompts))
	for _, rp := range s.prompts {
		defs = append(defs, rp.Def)
	}
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"prompts": defs},
	}
}

func (s *MCPServer) handlePromptsGet(req JSONRPCRequest) JSONRPCResponse {
	var params struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32602, Message: "invalid params: " + err.Error()},
		}
	}

	rp, ok := s.prompts[params.Name]
	if !ok {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32602, Message: fmt.Sprintf("unknown prompt: %s", params.Name)},
		}
	}

	text, err := rp.Render(params.Arguments)
	if err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32602, Message: err.Error()},
		}
	}

	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"description": rp.Def.Description,
			"messages": []map[string]interface{}{
				{
					"role": "user",
					"content": map[string]interface{}{
						"type": "text",
						"text": text,
					},
				},
			},
		},
	}
}

func writeResponse(w io.Writer, resp JSONRPCResponse) {
	data, _ := json.Marshal(resp)
	fmt.Fprintf(w, "%s\n", data)
}

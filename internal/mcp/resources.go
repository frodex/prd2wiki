package mcp

import (
	"fmt"
	"strings"
)

// registerResources sets up MCP resource handlers on the server.
//
// Supported resources:
//   - wiki://{project}/index  — list all pages in a project
//   - wiki://{project}/{id}   — read a specific page
func (s *MCPServer) registerResources() {
	s.RegisterResource("index", s.resourceIndex)
	s.RegisterResource("page", s.resourcePage)
}

// resourceIndex handles wiki://{project}/index — returns all pages.
func (s *MCPServer) resourceIndex(pathInfo string) (interface{}, error) {
	// pathInfo is "project/index"
	project := strings.SplitN(pathInfo, "/", 2)[0]
	pages, err := s.client.ListPages(project, nil)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	return pages, nil
}

// resourcePage handles wiki://{project}/{id} — returns a single page.
func (s *MCPServer) resourcePage(pathInfo string) (interface{}, error) {
	// pathInfo is "project/page-id"
	parts := strings.SplitN(pathInfo, "/", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid path: expected project/id")
	}
	project := parts[0]
	id := parts[1]

	page, err := s.client.GetPage(project, id, "")
	if err != nil {
		return nil, fmt.Errorf("get page: %w", err)
	}
	return page, nil
}

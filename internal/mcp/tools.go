package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// registerTools sets up all MCP tool handlers on the server.
func (s *MCPServer) registerTools() {
	s.RegisterTool(ToolDef{
		Name:        "wiki_search",
		Description: "Search wiki pages by query, type, status, or tag",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{"type": "string"},
				"query":   map[string]interface{}{"type": "string"},
				"type":    map[string]interface{}{"type": "string"},
				"status":  map[string]interface{}{"type": "string"},
				"tag":     map[string]interface{}{"type": "string"},
			},
			"required": []string{"project"},
		},
	}, s.toolSearch)

	s.RegisterTool(ToolDef{
		Name:        "wiki_read",
		Description: "Read a specific wiki page",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{"type": "string"},
				"id":      map[string]interface{}{"type": "string"},
				"branch":  map[string]interface{}{"type": "string"},
			},
			"required": []string{"project", "id"},
		},
	}, s.toolRead)

	s.RegisterTool(ToolDef{
		Name:        "wiki_propose",
		Description: "Create or update a wiki page on a draft branch",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{"type": "string"},
				"id":      map[string]interface{}{"type": "string"},
				"title":   map[string]interface{}{"type": "string"},
				"type":    map[string]interface{}{"type": "string"},
				"body":    map[string]interface{}{"type": "string"},
				"tags":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"intent":  map[string]interface{}{"type": "string", "enum": []string{"verbatim", "conform", "integrate"}},
			},
			"required": []string{"project", "id", "title", "type", "body"},
		},
	}, s.toolPropose)

	s.RegisterTool(ToolDef{
		Name:        "wiki_challenge",
		Description: "Challenge a wiki page with evidence of errors or contradictions",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project":   map[string]interface{}{"type": "string"},
				"target_id": map[string]interface{}{"type": "string"},
				"evidence":  map[string]interface{}{"type": "string"},
				"reason":    map[string]interface{}{"type": "string"},
			},
			"required": []string{"project", "target_id", "evidence", "reason"},
		},
	}, s.toolChallenge)

	s.RegisterTool(ToolDef{
		Name:        "wiki_ingest",
		Description: "Submit external source material for wiki ingestion",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{"type": "string"},
				"title":   map[string]interface{}{"type": "string"},
				"url":     map[string]interface{}{"type": "string"},
				"kind":    map[string]interface{}{"type": "string"},
				"content": map[string]interface{}{"type": "string"},
			},
			"required": []string{"project", "title", "content"},
		},
	}, s.toolIngest)

	s.RegisterTool(ToolDef{
		Name:        "wiki_lint",
		Description: "Check a page's provenance chain for stale or broken references",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{"type": "string"},
				"id":      map[string]interface{}{"type": "string"},
			},
			"required": []string{"project", "id"},
		},
	}, s.toolLint)

	s.RegisterTool(ToolDef{
		Name:        "wiki_status",
		Description: "Get status of a page or project overview",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{"type": "string"},
				"id":      map[string]interface{}{"type": "string"},
			},
			"required": []string{"project"},
		},
	}, s.toolStatus)
}

// ---------------------------------------------------------------------------
// Tool handler implementations
// ---------------------------------------------------------------------------

func (s *MCPServer) toolSearch(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Project string `json:"project"`
		Query   string `json:"query"`
		Type    string `json:"type"`
		Status  string `json:"status"`
		Tag     string `json:"tag"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Project == "" {
		return nil, fmt.Errorf("project is required")
	}

	params := make(map[string]string)
	if p.Query != "" {
		params["q"] = p.Query
	}
	if p.Type != "" {
		params["type"] = p.Type
	}
	if p.Status != "" {
		params["status"] = p.Status
	}
	if p.Tag != "" {
		params["tag"] = p.Tag
	}

	return s.client.Search(p.Project, params)
}

func (s *MCPServer) toolRead(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Project string `json:"project"`
		ID      string `json:"id"`
		Branch  string `json:"branch"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Project == "" || p.ID == "" {
		return nil, fmt.Errorf("project and id are required")
	}

	return s.client.GetPage(p.Project, p.ID, p.Branch)
}

func (s *MCPServer) toolPropose(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Project string   `json:"project"`
		ID      string   `json:"id"`
		Title   string   `json:"title"`
		Type    string   `json:"type"`
		Body    string   `json:"body"`
		Tags    []string `json:"tags"`
		Intent  string   `json:"intent"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Project == "" || p.ID == "" || p.Title == "" || p.Type == "" || p.Body == "" {
		return nil, fmt.Errorf("project, id, title, type, and body are required")
	}

	req := CreatePageRequest{
		ID:     p.ID,
		Title:  p.Title,
		Type:   p.Type,
		Body:   p.Body,
		Tags:   p.Tags,
		Branch: "draft/agent",
		Intent: p.Intent,
		Author: "mcp-agent",
	}

	return s.client.CreatePage(p.Project, req)
}

func (s *MCPServer) toolChallenge(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Project  string `json:"project"`
		TargetID string `json:"target_id"`
		Evidence string `json:"evidence"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Project == "" || p.TargetID == "" || p.Evidence == "" || p.Reason == "" {
		return nil, fmt.Errorf("project, target_id, evidence, and reason are required")
	}

	// Build a challenge page body in markdown.
	body := fmt.Sprintf("# Challenge: %s\n\n## Reason\n\n%s\n\n## Evidence\n\n%s\n",
		p.TargetID, p.Reason, p.Evidence)

	req := CreatePageRequest{
		ID:     fmt.Sprintf("challenge-%s", p.TargetID),
		Title:  fmt.Sprintf("Challenge: %s", p.TargetID),
		Type:   "challenge",
		Body:   body,
		Branch: fmt.Sprintf("challenge/%s", p.TargetID),
		Status: "contested",
		Author: "mcp-agent",
	}

	return s.client.CreatePage(p.Project, req)
}

func (s *MCPServer) toolIngest(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Project string `json:"project"`
		Title   string `json:"title"`
		URL     string `json:"url"`
		Kind    string `json:"kind"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Project == "" || p.Title == "" || p.Content == "" {
		return nil, fmt.Errorf("project, title, and content are required")
	}

	// Generate an ID from the title.
	id := "src-" + slugify(p.Title)

	// Build body with optional metadata header.
	var bodyParts []string
	bodyParts = append(bodyParts, fmt.Sprintf("# %s\n", p.Title))
	if p.URL != "" {
		bodyParts = append(bodyParts, fmt.Sprintf("Source URL: %s\n", p.URL))
	}
	if p.Kind != "" {
		bodyParts = append(bodyParts, fmt.Sprintf("Kind: %s\n", p.Kind))
	}
	bodyParts = append(bodyParts, fmt.Sprintf("\n%s\n", p.Content))

	req := CreatePageRequest{
		ID:     id,
		Title:  p.Title,
		Type:   "source",
		Body:   strings.Join(bodyParts, ""),
		Branch: "ingest/sources",
		Author: "mcp-agent",
	}

	return s.client.CreatePage(p.Project, req)
}

func (s *MCPServer) toolLint(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Project string `json:"project"`
		ID      string `json:"id"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Project == "" || p.ID == "" {
		return nil, fmt.Errorf("project and id are required")
	}

	refs, err := s.client.GetReferences(p.Project, p.ID, 3)
	if err != nil {
		return nil, fmt.Errorf("get references: %w", err)
	}

	// Walk the reference tree looking for problematic statuses.
	type Issue struct {
		Ref    string `json:"ref"`
		Status string `json:"status"`
		Reason string `json:"reason"`
	}

	var issues []Issue
	var walk func(nodes []RefNode)
	walk = func(nodes []RefNode) {
		for _, n := range nodes {
			switch n.Status {
			case "contested", "stale", "broken", "missing":
				issues = append(issues, Issue{
					Ref:    n.Ref,
					Status: n.Status,
					Reason: fmt.Sprintf("reference %s has status %q", n.Ref, n.Status),
				})
			}
			walk(n.Children)
		}
	}
	walk(refs.Hard)

	return map[string]interface{}{
		"page_id": p.ID,
		"ok":      len(issues) == 0,
		"issues":  issues,
	}, nil
}

func (s *MCPServer) toolStatus(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Project string `json:"project"`
		ID      string `json:"id"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Project == "" {
		return nil, fmt.Errorf("project is required")
	}

	// If a specific page ID is given, return its metadata.
	if p.ID != "" {
		page, err := s.client.GetPage(p.Project, p.ID, "")
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"id":          page.ID,
			"title":       page.Title,
			"type":        page.Type,
			"status":      page.Status,
			"trust_level": page.TrustLevel,
			"tags":        page.Tags,
		}, nil
	}

	// Otherwise return a project summary: counts by status and type.
	pages, err := s.client.ListPages(p.Project, nil)
	if err != nil {
		return nil, err
	}

	byStatus := make(map[string]int)
	byType := make(map[string]int)
	for _, pg := range pages {
		byStatus[pg.Status]++
		byType[pg.Type]++
	}

	return map[string]interface{}{
		"project":    p.Project,
		"total":      len(pages),
		"by_status":  byStatus,
		"by_type":    byType,
	}, nil
}

// slugify converts a title to a URL-friendly ID component.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, s)
	// Collapse runs of hyphens.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

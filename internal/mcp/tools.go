package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/frodex/prd2wiki/internal/tree"
)

// registerTools sets up all MCP tool handlers on the server.
func (s *MCPServer) registerTools() {
	s.RegisterTool(ToolDef{
		Name:        "wiki_search",
		Description: "Search wiki pages by query, type, status, or tag. Use optional path to limit results to a tree subtree.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{"type": "string", "description": "Wiki project key (repo key), e.g. default"},
				"path":    map[string]interface{}{"type": "string", "description": "Optional tree URL prefix to filter results (e.g. prd2wiki/docs)"},
				"query":   map[string]interface{}{"type": "string"},
				"type":    map[string]interface{}{"type": "string"},
				"status":  map[string]interface{}{"type": "string"},
				"tag":     map[string]interface{}{"type": "string"},
			},
		},
	}, s.toolSearch)

	s.RegisterTool(ToolDef{
		Name:        "wiki_read",
		Description: "Read a wiki page by legacy project+id, or by tree path (e.g. prd2wiki/my-page) or page UUID.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{"type": "string"},
				"id":      map[string]interface{}{"type": "string"},
				"path":    map[string]interface{}{"type": "string", "description": "Tree URL path or bare page UUID"},
				"branch":  map[string]interface{}{"type": "string"},
			},
		},
	}, s.toolRead)

	s.RegisterTool(ToolDef{
		Name:        "wiki_propose",
		Description: "Create or update a wiki page on a draft branch. Use path for tree placement; a .link file is written when the tree index is configured.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{"type": "string"},
				"path":    map[string]interface{}{"type": "string", "description": "Tree location for the new page (e.g. prd2wiki or prd2wiki/my-slug)"},
				"id":      map[string]interface{}{"type": "string"},
				"title":   map[string]interface{}{"type": "string"},
				"type":    map[string]interface{}{"type": "string"},
				"body":    map[string]interface{}{"type": "string"},
				"tags":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"intent":  map[string]interface{}{"type": "string", "enum": []string{"verbatim", "conform", "integrate"}},
			},
			"required": []string{"title", "type", "body"},
		},
	}, s.toolPropose)

	s.RegisterTool(ToolDef{
		Name:        "wiki_challenge",
		Description: "Challenge a wiki page with evidence. Use target_path or legacy project+target_id.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project":     map[string]interface{}{"type": "string"},
				"target_id":   map[string]interface{}{"type": "string"},
				"target_path": map[string]interface{}{"type": "string", "description": "Tree path of the page to challenge"},
				"evidence":    map[string]interface{}{"type": "string"},
				"reason":      map[string]interface{}{"type": "string"},
			},
			"required": []string{"evidence", "reason"},
		},
	}, s.toolChallenge)

	s.RegisterTool(ToolDef{
		Name:        "wiki_ingest",
		Description: "Submit external source material for wiki ingestion. Use path to choose tree project scope.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{"type": "string"},
				"path":    map[string]interface{}{"type": "string", "description": "Tree path whose project receives the page (e.g. prd2wiki)"},
				"title":   map[string]interface{}{"type": "string"},
				"url":     map[string]interface{}{"type": "string"},
				"kind":    map[string]interface{}{"type": "string"},
				"content": map[string]interface{}{"type": "string"},
			},
			"required": []string{"title", "content"},
		},
	}, s.toolIngest)

	s.RegisterTool(ToolDef{
		Name:        "wiki_lint",
		Description: "Check a page's provenance chain. Use path or legacy project+id.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{"type": "string"},
				"id":      map[string]interface{}{"type": "string"},
				"path":    map[string]interface{}{"type": "string"},
			},
		},
	}, s.toolLint)

	s.RegisterTool(ToolDef{
		Name:        "wiki_status",
		Description: "Status for a page or project overview. Use path (tree URL) or legacy project (+ optional id).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{"type": "string"},
				"id":      map[string]interface{}{"type": "string"},
				"path":    map[string]interface{}{"type": "string"},
			},
		},
	}, s.toolStatus)

	s.RegisterTool(ToolDef{
		Name:        "wiki_move",
		Description: "Move a page's .link to a new tree URL (temporary redirect at the old URL). Requires local tree root.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"from": map[string]interface{}{"type": "string", "description": "Current tree path, e.g. prd2wiki/old-slug"},
				"to":   map[string]interface{}{"type": "string", "description": "Destination tree path"},
			},
			"required": []string{"from", "to"},
		},
	}, s.toolMove)

	s.RegisterTool(ToolDef{
		Name:        "wiki_rename",
		Description: "Rename a page slug within its parent directory (permanent redirect at the old URL). Requires local tree root.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path":      map[string]interface{}{"type": "string", "description": "Full tree path to the page, e.g. prd2wiki/foo"},
				"new_slug":  map[string]interface{}{"type": "string", "description": "New final path segment (slug)"},
			},
			"required": []string{"path", "new_slug"},
		},
	}, s.toolRename)
}

// ---------------------------------------------------------------------------
// Tree helpers
// ---------------------------------------------------------------------------

func (s *MCPServer) requireTree() (*tree.Index, error) {
	if s.treeHolder == nil {
		return nil, fmt.Errorf("tree index not configured (set PRDWIKI_TREE_ROOT and PRDWIKI_DATA_DIR)")
	}
	idx := s.treeHolder.Get()
	if idx == nil {
		return nil, fmt.Errorf("tree index not available")
	}
	return idx, nil
}

func longestProjectPrefix(idx *tree.Index, urlPath string) (*tree.Project, string) {
	urlPath = strings.Trim(urlPath, "/")
	if urlPath == "" || idx == nil {
		return nil, ""
	}
	var best *tree.Project
	for _, p := range idx.Projects {
		if p == nil {
			continue
		}
		if urlPath == p.Path {
			return p, ""
		}
		if strings.HasPrefix(urlPath, p.Path+"/") {
			if best == nil || len(p.Path) > len(best.Path) {
				best = p
			}
		}
	}
	if best == nil {
		return nil, ""
	}
	rest := strings.TrimPrefix(urlPath[len(best.Path):], "/")
	return best, rest
}

func (s *MCPServer) resolvePageEntry(path string) (*tree.PageEntry, error) {
	idx, err := s.requireTree()
	if err != nil {
		return nil, err
	}
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "/")
	if path == "" {
		return nil, fmt.Errorf("empty path")
	}
	if parsed, err := uuid.Parse(path); err == nil {
		if e, ok := idx.PageByUUID(parsed.String()); ok {
			return e, nil
		}
		return nil, fmt.Errorf("no page for UUID %s", path)
	}
	if e, ok := idx.PageByURLPath(path); ok {
		return e, nil
	}
	return nil, fmt.Errorf("unknown tree path or page UUID %q", path)
}

func treePathHasPrefix(full, prefix string) bool {
	prefix = strings.Trim(prefix, "/")
	full = strings.Trim(full, "/")
	if prefix == "" {
		return true
	}
	return full == prefix || strings.HasPrefix(full, prefix+"/")
}

// ---------------------------------------------------------------------------
// Tool handler implementations
// ---------------------------------------------------------------------------

func (s *MCPServer) toolSearch(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Project string `json:"project"`
		Path    string `json:"path"`
		Query   string `json:"query"`
		Type    string `json:"type"`
		Status  string `json:"status"`
		Tag     string `json:"tag"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	scopePrefix := strings.Trim(p.Path, "/")
	repoKey := strings.TrimSpace(p.Project)

	if repoKey == "" && scopePrefix != "" {
		idx, err := s.requireTree()
		if err != nil {
			return nil, err
		}
		proj, _ := longestProjectPrefix(idx, scopePrefix)
		if proj == nil {
			return nil, fmt.Errorf("path does not match any tree project: %q", p.Path)
		}
		repoKey = proj.RepoKey
	}
	if repoKey == "" {
		return nil, fmt.Errorf("project or path is required")
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

	results, err := s.client.Search(repoKey, params)
	if err != nil {
		return nil, err
	}
	if scopePrefix == "" {
		return results, nil
	}
	idx, err := s.requireTree()
	if err != nil {
		return nil, err
	}
	var out []PageResult
	for _, r := range results {
		e, ok := idx.PageByUUID(r.ID)
		if !ok {
			continue
		}
		if treePathHasPrefix(e.Page.TreePath, scopePrefix) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *MCPServer) toolRead(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Project string `json:"project"`
		ID      string `json:"id"`
		Path    string `json:"path"`
		Branch  string `json:"branch"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if strings.TrimSpace(p.Path) != "" {
		ent, err := s.resolvePageEntry(p.Path)
		if err != nil {
			return nil, err
		}
		return s.client.GetPage(ent.Project.RepoKey, ent.Page.UUID, p.Branch)
	}
	if p.Project != "" && p.ID != "" {
		return s.client.GetPage(p.Project, p.ID, p.Branch)
	}
	return nil, fmt.Errorf("provide path, or project and id")
}

func (s *MCPServer) toolPropose(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Project string   `json:"project"`
		Path    string   `json:"path"`
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
	if p.Title == "" || p.Type == "" || p.Body == "" {
		return nil, fmt.Errorf("title, type, and body are required")
	}

	locPath := strings.TrimSpace(p.Path)
	repoKey := strings.TrimSpace(p.Project)

	if locPath != "" {
		idx, err := s.requireTree()
		if err != nil {
			return nil, err
		}
		proj, rest := longestProjectPrefix(idx, locPath)
		if proj == nil {
			return nil, fmt.Errorf("path does not match any tree project: %q", locPath)
		}
		repoKey = proj.RepoKey

		var fullTreeURL string
		if rest == "" && strings.TrimSuffix(locPath, "/") == proj.Path {
			base := tree.SlugFromTitle(p.Title)
			used := idx.UsedSlugs(proj.Path)
			slug := tree.UniqueSlug(base, used)
			fullTreeURL = proj.Path + "/" + slug
		} else {
			fullTreeURL = strings.Trim(locPath, "/")
		}

		id := strings.TrimSpace(p.ID)
		if id == "" {
			id = tree.SlugFromTitle(p.Title)
		}

		req := CreatePageRequest{
			ID:     id,
			Title:  p.Title,
			Type:   p.Type,
			Body:   p.Body,
			Tags:   p.Tags,
			Branch: "draft/incoming",
			Intent: p.Intent,
			Author: "mcp-agent",
		}

		resp, err := s.client.CreatePage(repoKey, req)
		if err != nil {
			return nil, err
		}
		pageUUID := strings.TrimSpace(resp.ID)
		if pageUUID == "" {
			pageUUID = id
		}
		if err := tree.WriteLinkFileAtTreeURL(s.treeHolder.TreeRoot(), fullTreeURL, pageUUID, p.Title); err != nil {
			return nil, fmt.Errorf("write .link: %w", err)
		}
		if err := s.treeHolder.Refresh(); err != nil {
			return nil, fmt.Errorf("tree refresh: %w", err)
		}
		out := map[string]interface{}{
			"id":         pageUUID,
			"title":      resp.Title,
			"status":     resp.Status,
			"path":       resp.Path,
			"issues":     resp.Issues,
			"warnings":   resp.Warnings,
			"valid":      resp.Valid,
			"tree_path":  fullTreeURL,
			"url":        "/" + fullTreeURL,
			"project":    repoKey,
		}
		return out, nil
	}

	if repoKey == "" {
		return nil, fmt.Errorf("project or path is required")
	}

	req := CreatePageRequest{
		ID:     p.ID,
		Title:  p.Title,
		Type:   p.Type,
		Body:   p.Body,
		Tags:   p.Tags,
		Branch: "draft/incoming",
		Intent: p.Intent,
		Author: "mcp-agent",
	}

	return s.client.CreatePage(repoKey, req)
}

func (s *MCPServer) toolChallenge(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Project     string `json:"project"`
		TargetID    string `json:"target_id"`
		TargetPath  string `json:"target_path"`
		Evidence    string `json:"evidence"`
		Reason      string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Evidence == "" || p.Reason == "" {
		return nil, fmt.Errorf("evidence and reason are required")
	}

	targetID := strings.TrimSpace(p.TargetID)
	if strings.TrimSpace(p.TargetPath) != "" {
		ent, err := s.resolvePageEntry(p.TargetPath)
		if err != nil {
			return nil, err
		}
		targetID = ent.Page.UUID
		p.Project = ent.Project.RepoKey
	}
	if p.Project == "" || targetID == "" {
		return nil, fmt.Errorf("target_path or project and target_id are required")
	}

	body := fmt.Sprintf("# Challenge: %s\n\n## Reason\n\n%s\n\n## Evidence\n\n%s\n",
		targetID, p.Reason, p.Evidence)

	req := CreatePageRequest{
		ID:     fmt.Sprintf("challenge-%s", targetID),
		Title:  fmt.Sprintf("Challenge: %s", targetID),
		Type:   "challenge",
		Body:   body,
		Branch: fmt.Sprintf("challenge/%s", targetID),
		Status: "contested",
		Author: "mcp-agent",
	}

	return s.client.CreatePage(p.Project, req)
}

func (s *MCPServer) toolIngest(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Project string `json:"project"`
		Path    string `json:"path"`
		Title   string `json:"title"`
		URL     string `json:"url"`
		Kind    string `json:"kind"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Title == "" || p.Content == "" {
		return nil, fmt.Errorf("title and content are required")
	}

	repoKey := strings.TrimSpace(p.Project)
	if strings.TrimSpace(p.Path) != "" {
		idx, err := s.requireTree()
		if err != nil {
			return nil, err
		}
		proj, _ := longestProjectPrefix(idx, strings.TrimSpace(p.Path))
		if proj == nil {
			return nil, fmt.Errorf("path does not match any tree project: %q", p.Path)
		}
		repoKey = proj.RepoKey
	}
	if repoKey == "" {
		return nil, fmt.Errorf("project or path is required")
	}

	id := "src-" + slugify(p.Title)

	// BUG-005: allow caller to override type via 'kind' field, default "source"
	pageType := "source"
	if p.Kind != "" {
		pageType = p.Kind
	}

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
		Type:   pageType,
		Body:   strings.Join(bodyParts, ""),
		Branch: "ingest/sources",
		Author: "mcp-agent",
	}

	return s.client.CreatePage(repoKey, req)
}

func (s *MCPServer) toolLint(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Project string `json:"project"`
		ID      string `json:"id"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	repoKey := strings.TrimSpace(p.Project)
	pageID := strings.TrimSpace(p.ID)
	if strings.TrimSpace(p.Path) != "" {
		ent, err := s.resolvePageEntry(p.Path)
		if err != nil {
			return nil, err
		}
		repoKey = ent.Project.RepoKey
		pageID = ent.Page.UUID
	}
	if repoKey == "" || pageID == "" {
		return nil, fmt.Errorf("path or project and id are required")
	}

	refs, err := s.client.GetReferences(repoKey, pageID, 3)
	if err != nil {
		return nil, fmt.Errorf("get references: %w", err)
	}

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
		"page_id": pageID,
		"ok":      len(issues) == 0,
		"issues":  issues,
	}, nil
}

func (s *MCPServer) toolStatus(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Project string `json:"project"`
		ID      string `json:"id"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if strings.TrimSpace(p.Path) != "" {
		idx, err := s.requireTree()
		if err != nil {
			return nil, err
		}
		path := strings.Trim(p.Path, "/")
		if ent, ok := idx.PageByURLPath(path); ok {
			page, err := s.client.GetPage(ent.Project.RepoKey, ent.Page.UUID, "")
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
				"tree_path":   ent.Page.TreePath,
			}, nil
		}
		if proj, ok := idx.ProjectByTreePath(path); ok {
			return s.treeProjectSummary(proj.RepoKey, idx, proj.Path)
		}
		proj, rest := longestProjectPrefix(idx, path)
		if proj != nil && rest == "" {
			return s.treeProjectSummary(proj.RepoKey, idx, proj.Path)
		}
		if proj != nil {
			return s.treeSubtreeSummary(proj.RepoKey, idx, path)
		}
		return nil, fmt.Errorf("unknown tree path %q", p.Path)
	}

	if p.Project == "" {
		return nil, fmt.Errorf("project or path is required")
	}

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
		"project":   p.Project,
		"total":     len(pages),
		"by_status": byStatus,
		"by_type":   byType,
	}, nil
}

func (s *MCPServer) treeProjectSummary(repoKey string, idx *tree.Index, projPath string) (map[string]interface{}, error) {
	pages, err := s.client.ListPages(repoKey, nil)
	if err != nil {
		return nil, err
	}
	byStatus := make(map[string]int)
	byType := make(map[string]int)
	treeCount := 0
	for _, pg := range pages {
		byStatus[pg.Status]++
		byType[pg.Type]++
	}
	for _, e := range idx.AllPageEntries() {
		if e.Project.Path == projPath {
			treeCount++
		}
	}
	return map[string]interface{}{
		"project":     repoKey,
		"tree_path":   projPath,
		"total":       len(pages),
		"tree_pages":  treeCount,
		"by_status":   byStatus,
		"by_type":     byType,
	}, nil
}

func (s *MCPServer) treeSubtreeSummary(repoKey string, idx *tree.Index, prefix string) (map[string]interface{}, error) {
	prefix = strings.Trim(prefix, "/")
	var inTree int
	for _, e := range idx.AllPageEntries() {
		if e.Project.RepoKey != repoKey {
			continue
		}
		if treePathHasPrefix(e.Page.TreePath, prefix) {
			inTree++
		}
	}
	return map[string]interface{}{
		"project":          repoKey,
		"tree_path_prefix": prefix,
		"pages_in_subtree": inTree,
	}, nil
}

func (s *MCPServer) toolMove(raw json.RawMessage) (interface{}, error) {
	var p struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	from := strings.Trim(p.From, "/")
	to := strings.Trim(p.To, "/")
	if from == "" || to == "" {
		return nil, fmt.Errorf("from and to are required")
	}
	if s.treeHolder == nil {
		return nil, fmt.Errorf("tree not configured")
	}
	root := s.treeHolder.TreeRoot()
	if err := tree.MovePage(root, from, to); err != nil {
		return nil, err
	}
	loc := "/" + to
	if err := tree.WriteLeafRedirect(root, from, loc, false); err != nil {
		return nil, err
	}
	if err := s.treeHolder.Refresh(); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"from":        from,
		"to":          to,
		"redirect":    loc,
		"redirect_ok": true,
	}, nil
}

func (s *MCPServer) toolRename(raw json.RawMessage) (interface{}, error) {
	var p struct {
		Path    string `json:"path"`
		NewSlug string `json:"new_slug"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	path := strings.Trim(p.Path, "/")
	newSlug := strings.TrimSpace(p.NewSlug)
	if path == "" || newSlug == "" {
		return nil, fmt.Errorf("path and new_slug are required")
	}
	if s.treeHolder == nil {
		return nil, fmt.Errorf("tree not configured")
	}
	idx, err := s.requireTree()
	if err != nil {
		return nil, err
	}
	if _, ok := idx.PageByURLPath(path); !ok {
		return nil, fmt.Errorf("page not found at tree path %q", path)
	}
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return nil, fmt.Errorf("invalid path %q", path)
	}
	projectRel := path[:i]
	oldSlug := path[i+1:]
	newPath := projectRel + "/" + newSlug
	root := s.treeHolder.TreeRoot()
	if err := tree.RenamePage(root, projectRel, oldSlug, newSlug); err != nil {
		return nil, err
	}
	if err := tree.WriteLeafRedirect(root, path, "/"+newPath, true); err != nil {
		return nil, err
	}
	if err := s.treeHolder.Refresh(); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"path":     path,
		"new_path": newPath,
		"url":      "/" + newPath,
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
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

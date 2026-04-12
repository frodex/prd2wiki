package web

import (
	"net/http"
	"sort"
	"strings"

	"github.com/frodex/prd2wiki/internal/index"
)

// PageListItem represents one row in the page listing table.
type PageListItem struct {
	ID           string
	Title        string
	Type         string
	Status       string
	TrustLevel   int
	Path         string
	Module       string
	Category     string
	LastEditBy   string
	LastEditDate string
	Score        string // similarity score for search results
}

// ModuleGroup groups page list items under a module heading.
type ModuleGroup struct {
	Module string
	Items  []PageListItem
}

// TreeNode represents a node in the sidebar navigation tree.
type TreeNode struct {
	Name     string
	Path     string // filter path: "docs/plans"
	Children []TreeNode
	Count    int  // number of pages in this branch
	Active   bool // currently selected
}

// PageListData holds tree + grouped items for the page list template.
type PageListData struct {
	Tree       []TreeNode
	Groups     []ModuleGroup
	TreeFilter string
}

// listPages renders the page listing for a project.
func (h *Handler) listPages(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	treeFilter := r.URL.Query().Get("tree")
	tagFilter := r.URL.Query().Get("tag")

	var results []index.PageResult
	var err error
	if tagFilter != "" {
		results, err = h.search.ByTag(project, tagFilter)
	} else {
		results, err = h.search.ListAll(project)
	}
	if err != nil {
		http.Error(w, "failed to list pages: "+err.Error(), http.StatusInternalServerError)
		return
	}

	cache := h.edits[project]

	allItems := make([]PageListItem, len(results))
	for i, pr := range results {
		allItems[i] = PageListItem{
			ID:         pr.ID,
			Title:      pr.Title,
			Type:       pr.Type,
			Status:     pr.Status,
			TrustLevel: pr.TrustLevel,
			Path:       pr.Path,
			Module:     pr.Module,
			Category:   pr.Category,
		}
		if cache != nil {
			if info, ok := cache.Get(pr.Path); ok {
				allItems[i].LastEditBy = info.Author
				allItems[i].LastEditDate = info.Date
			}
		}
	}

	// Build tree from all items (before filtering).
	tree := buildTree(allItems, treeFilter)

	// Filter items by tree selection (directory prefix match on path).
	var filtered []PageListItem
	if treeFilter == "" {
		filtered = allItems
	} else {
		prefix := "pages/" + treeFilter + "/"
		for _, item := range allItems {
			if strings.HasPrefix(item.Path, prefix) {
				filtered = append(filtered, item)
			}
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].LastEditDate > filtered[j].LastEditDate
	})

	// Group filtered items by module.
	moduleOrder := []string{}
	moduleMap := make(map[string][]PageListItem)
	for _, item := range filtered {
		mod := item.Module
		if mod == "" {
			mod = "Other"
		}
		if _, exists := moduleMap[mod]; !exists {
			moduleOrder = append(moduleOrder, mod)
		}
		moduleMap[mod] = append(moduleMap[mod], item)
	}
	groups := make([]ModuleGroup, len(moduleOrder))
	for i, mod := range moduleOrder {
		groups[i] = ModuleGroup{Module: mod, Items: moduleMap[mod]}
	}

	pld := PageListData{
		Tree:       tree,
		Groups:     groups,
		TreeFilter: treeFilter,
	}

	data := PageData{
		Project:  project,
		Title:    project + " — Pages",
		Content:  pld,
		Projects: h.projects(),
	}

	t := h.templates["templates/page_list.html"]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// pathToTree splits a page path into directory segments and a filename.
// "pages/docs/research/mechlab.md" → (["docs","research"], "mechlab.md")
// "pages/DESIGN-003.md" → (nil, "DESIGN-003.md")
func pathToTree(path string) (dirs []string, filename string) {
	path = strings.TrimPrefix(path, "pages/")
	parts := strings.Split(path, "/")
	if len(parts) == 1 {
		return nil, parts[0]
	}
	return parts[:len(parts)-1], parts[len(parts)-1]
}

// buildTree constructs a directory-based tree from page paths.
func buildTree(items []PageListItem, activeFilter string) []TreeNode {
	// Count pages per directory path.
	dirCounts := map[string]int{}
	dirChildren := map[string]map[string]bool{} // parent → set of immediate child dir names

	for _, item := range items {
		dirs, _ := pathToTree(item.Path)
		if len(dirs) == 0 {
			// Root-level page — no directory node needed, counted in "All".
			continue
		}
		// Count this page at every ancestor directory level.
		for i := range dirs {
			dirPath := strings.Join(dirs[:i+1], "/")
			dirCounts[dirPath]++
			// Track parent→child relationships.
			parentPath := ""
			if i > 0 {
				parentPath = strings.Join(dirs[:i], "/")
			}
			if dirChildren[parentPath] == nil {
				dirChildren[parentPath] = map[string]bool{}
			}
			dirChildren[parentPath][dirs[i]] = true
		}
	}

	tree := []TreeNode{{
		Name:   "All",
		Path:   "",
		Count:  len(items),
		Active: activeFilter == "",
	}}

	// Recursively build tree nodes from the root level.
	var buildNodes func(parentPath string) []TreeNode
	buildNodes = func(parentPath string) []TreeNode {
		children, ok := dirChildren[parentPath]
		if !ok {
			return nil
		}
		// Sorted child names.
		names := make([]string, 0, len(children))
		for name := range children {
			names = append(names, name)
		}
		sort.Strings(names)

		var nodes []TreeNode
		for _, name := range names {
			var nodePath string
			if parentPath == "" {
				nodePath = name
			} else {
				nodePath = parentPath + "/" + name
			}
			node := TreeNode{
				Name:     name,
				Path:     nodePath,
				Count:    dirCounts[nodePath],
				Active:   activeFilter == nodePath,
				Children: buildNodes(nodePath),
			}
			nodes = append(nodes, node)
		}
		return nodes
	}

	tree = append(tree, buildNodes("")...)
	return tree
}

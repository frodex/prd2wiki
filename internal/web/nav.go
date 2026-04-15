package web

import (
	"sort"
	"strings"

	"github.com/frodex/prd2wiki/internal/tree"
)

// Breadcrumb is one segment in the location bar.
type Breadcrumb struct {
	Label string
	Href  string // empty = current page (not a link)
}

// TreeNavData is the left wiki sidebar built from the on-disk tree index.
type TreeNavData struct {
	Projects []TreeNavProject
}

// TreeNavProject is one project folder under tree/ with its pages.
type TreeNavProject struct {
	Name  string
	Path  string // tree path, e.g. "prd2wiki"
	Pages []TreeNavPage
}

// TreeNavPage is a single page link in the sidebar.
type TreeNavPage struct {
	Title string
	Href  string
}

// TreeDirectoryData is the project root listing (GET /{treeProjectPath}).
type TreeDirectoryData struct {
	ProjectName string
	TreePath    string
	Pages       []TreeDirEntry
}

// TreeDirEntry is one row on the directory page.
type TreeDirEntry struct {
	Title string
	Slug  string
	Href  string
}

func (h *Handler) preparePageData(d *PageData) {
	d.Projects = h.projects()
	d.WriteToken = h.writeToken
	if h.treeHolder != nil {
		if idx := h.treeHolder.Get(); idx != nil {
			d.TreeNav = buildTreeSidebar(idx)
		}
	}
	if len(d.Breadcrumbs) == 0 {
		d.Breadcrumbs = []Breadcrumb{{Label: "Home", Href: "/"}}
	}
}

func buildTreeSidebar(idx *tree.Index) *TreeNavData {
	if idx == nil {
		return nil
	}
	projs := append([]*tree.Project(nil), idx.Projects...)
	sort.Slice(projs, func(i, j int) bool {
		return projs[i].Path < projs[j].Path
	})
	out := make([]TreeNavProject, 0, len(projs))
	for _, p := range projs {
		var pages []TreeNavPage
		for _, e := range idx.AllPageEntries() {
			if e.Project.Path != p.Path {
				continue
			}
			title := strings.TrimSpace(e.Page.Title)
			if title == "" {
				title = e.Page.Slug
			}
			pages = append(pages, TreeNavPage{
				Title: title,
				Href:  "/" + e.Page.TreePath,
			})
		}
		sort.Slice(pages, func(i, j int) bool {
			return strings.ToLower(pages[i].Title) < strings.ToLower(pages[j].Title)
		})
		out = append(out, TreeNavProject{
			Name:  p.Name,
			Path:  p.Path,
			Pages: pages,
		})
	}
	return &TreeNavData{Projects: out}
}

func treeBreadcrumbs(idx *tree.Index, urlPath, pageTitle string) []Breadcrumb {
	out := []Breadcrumb{{Label: "Home", Href: "/"}}
	if idx == nil {
		return out
	}
	urlPath = strings.Trim(urlPath, "/")
	if urlPath == "" {
		return out
	}
	segs := strings.Split(urlPath, "/")
	acc := ""
	for i, seg := range segs {
		if i == 0 {
			acc = seg
		} else {
			acc = acc + "/" + seg
		}
		last := i == len(segs)-1
		label := seg
		if p, ok := idx.ProjectByTreePath(acc); ok && strings.TrimSpace(p.Name) != "" {
			label = p.Name
		}
		if last {
			if strings.TrimSpace(pageTitle) != "" {
				label = pageTitle
			}
			out = append(out, Breadcrumb{Label: label, Href: ""})
		} else {
			out = append(out, Breadcrumb{Label: label, Href: "/" + acc})
		}
	}
	return out
}

func projectPageBreadcrumbs(project, title string) []Breadcrumb {
	return []Breadcrumb{
		{Label: "Home", Href: "/"},
		{Label: project, Href: "/projects/" + project + "/pages"},
		{Label: title, Href: ""},
	}
}

func projectSectionBreadcrumbs(project, sectionTitle string) []Breadcrumb {
	return []Breadcrumb{
		{Label: "Home", Href: "/"},
		{Label: project, Href: "/projects/" + project + "/pages"},
		{Label: sectionTitle, Href: ""},
	}
}

func (h *Handler) breadcrumbsForGitPage(project, pageUUID, title string) []Breadcrumb {
	if h.treeHolder != nil {
		idx := h.treeHolder.Get()
		if idx != nil {
			if ent, ok := idx.PageByUUID(strings.TrimSpace(pageUUID)); ok {
				return treeBreadcrumbs(idx, ent.URLPath(), title)
			}
		}
	}
	return projectPageBreadcrumbs(project, title)
}

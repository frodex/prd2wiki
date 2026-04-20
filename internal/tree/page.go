package tree

// Page is one .link file under a project tree directory.
type Page struct {
	UUID         string // line 1
	LibrarianID  string // line 2 (may be empty)
	Title        string // line 3
	Slug         string // filename without .link
	TreePath     string // relative path under tree root: "prd2wiki/my-page-slug" (no .link suffix)
}

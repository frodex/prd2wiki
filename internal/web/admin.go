package web

import (
	"net/http"

	"github.com/frodex/prd2wiki/internal/auth"
)

// ScopeAdmin is required for mutating /admin/* requests (POST, etc.).
const ScopeAdmin = "admin"

// AdminIndexData is the admin hub page.
type AdminIndexData struct {
	Links []AdminNavLink
}

// AdminNavLink is one entry on the admin index.
type AdminNavLink struct {
	Href        string
	Label       string
	Description string
}

// AdminStubData is a placeholder admin tool page (Phase 3d — CLI not wired yet).
type AdminStubData struct {
	Title       string
	Intro       string
	Note        string
	FormAction  string
	ButtonLabel string
}

func (h *Handler) adminIndex(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		Title: "Admin",
		Content: AdminIndexData{
			Links: []AdminNavLink{
				{Href: "/admin/export", Label: "Export", Description: "Export wiki data (stub)."},
				{Href: "/admin/import", Label: "Import", Description: "Import wiki data (stub)."},
				{Href: "/admin/verify", Label: "Verify", Description: "Verify tree and index consistency (stub)."},
			},
		},
		Breadcrumbs: []Breadcrumb{
			{Label: "Home", Href: "/"},
			{Label: "Admin", Href: ""},
		},
	}
	h.preparePageData(&data)
	t := h.templates["templates/admin_index.html"]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) adminExportGet(w http.ResponseWriter, r *http.Request) {
	h.adminStubGet(w, AdminStubData{
		Title:       "Export",
		Intro:       "Export a wiki bundle or snapshot. The real export CLI is not connected yet.",
		Note:        "POST requests require Authorization: Bearer … with scope " + ScopeAdmin + ".",
		FormAction:  "/admin/export",
		ButtonLabel: "Run export (stub)",
	}, []Breadcrumb{
		{Label: "Home", Href: "/"},
		{Label: "Admin", Href: "/admin"},
		{Label: "Export", Href: ""},
	})
}

func (h *Handler) adminImportGet(w http.ResponseWriter, r *http.Request) {
	h.adminStubGet(w, AdminStubData{
		Title:       "Import",
		Intro:       "Import a wiki bundle into this instance. The real import CLI is not connected yet.",
		Note:        "POST requests require Authorization: Bearer … with scope " + ScopeAdmin + ".",
		FormAction:  "/admin/import",
		ButtonLabel: "Run import (stub)",
	}, []Breadcrumb{
		{Label: "Home", Href: "/"},
		{Label: "Admin", Href: "/admin"},
		{Label: "Import", Href: ""},
	})
}

func (h *Handler) adminVerifyGet(w http.ResponseWriter, r *http.Request) {
	h.adminStubGet(w, AdminStubData{
		Title:       "Verify",
		Intro:       "Verify on-disk tree, git repos, and index consistency. Checks are not implemented yet.",
		Note:        "POST requests require Authorization: Bearer … with scope " + ScopeAdmin + ".",
		FormAction:  "/admin/verify",
		ButtonLabel: "Run verification (stub)",
	}, []Breadcrumb{
		{Label: "Home", Href: "/"},
		{Label: "Admin", Href: "/admin"},
		{Label: "Verify", Href: ""},
	})
}

func (h *Handler) adminStubGet(w http.ResponseWriter, content AdminStubData, crumbs []Breadcrumb) {
	data := PageData{
		Title:       "Admin — " + content.Title,
		Content:     content,
		Breadcrumbs: crumbs,
	}
	h.preparePageData(&data)
	t := h.templates["templates/admin_stub.html"]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) adminExportPost(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("export not implemented\n"))
}

func (h *Handler) adminImportPost(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("import not implemented\n"))
}

func (h *Handler) adminVerifyPost(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("verify not implemented\n"))
}

// wrapAdminMutating applies API-key auth with ScopeAdmin. If no key store is configured, mutating routes return 503.
func (h *Handler) wrapAdminMutating(next http.HandlerFunc) http.Handler {
	if h.keys == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "admin authentication is not configured", http.StatusServiceUnavailable)
		})
	}
	return auth.RequireAPIKey(h.keys, ScopeAdmin)(next)
}

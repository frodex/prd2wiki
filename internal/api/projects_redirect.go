package api

import (
	"net/http"
	"path"
	"strings"
)

const (
	headerDeprecation = "Deprecation"
	headerLink        = "Link"
)

// wrapProjectsAPILegacyRedirect returns a handler that 308-redirects legacy /api/projects/...
// to /api/tree/... when a tree mapping exists.
func (s *Server) wrapProjectsAPILegacyRedirect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.treeHolder == nil || s.treeHolder.Get() == nil {
			next.ServeHTTP(w, r)
			return
		}
		// Only redirect GETs — mutating requests from the web UI go through
		// the old handler directly (no auth required for same-origin writes).
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		if target, ok := s.mapLegacyProjectsAPIToTree(r); ok {
			q := r.URL.RawQuery
			if q != "" {
				if strings.Contains(target, "?") {
					target = target + "&" + q
				} else {
					target = target + "?" + q
				}
			}
			w.Header().Set(headerDeprecation, "true")
			w.Header().Set(headerLink, `<`+target+`>; rel="successor-version"`)
			http.Redirect(w, r, target, http.StatusPermanentRedirect)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// mapLegacyProjectsAPIToTree returns the new /api/tree path or ok=false.
func (s *Server) mapLegacyProjectsAPIToTree(r *http.Request) (string, bool) {
	p := path.Clean(r.URL.Path)
	prefix := "/api/projects/"
	if !strings.HasPrefix(p, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(p, prefix)
	if rest == "" || rest == "." {
		return "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 1 {
		return "", false
	}
	repoKey := parts[0]
	proj, ok := s.treeHolder.Get().ProjectByRepoKey(repoKey)
	if !ok {
		return "", false
	}
	treeP := proj.Path
	tail := strings.Join(parts[1:], "/")

	switch {
	case tail == "pages" && r.Method == http.MethodGet:
		return "/api/tree/" + treeP, true
	case tail == "pages" && r.Method == http.MethodPost:
		return "/api/tree/" + treeP + "/pages", true
	case strings.HasPrefix(tail, "pages/"):
		sub := strings.TrimPrefix(tail, "pages/")
		if sub == "" {
			return "", false
		}
		// attachments, history, etc. — only redirect simple page CRUD
		if strings.Contains(sub, "/") {
			// /pages/id/attachments/... — try map id to slug for first segment only
			segs := strings.SplitN(sub, "/", 2)
			id := segs[0]
			suffix := ""
			if len(segs) > 1 {
				suffix = segs[1]
			}
			rows, err := s.search.ByID(repoKey, id)
			if err != nil || len(rows) == 0 {
				return "", false
			}
			ent, ok := s.treeHolder.Get().PageByUUID(rows[0].ID)
			if !ok {
				return "", false
			}
			base := "/api/tree/" + ent.Page.TreePath
			if suffix == "" {
				return base, true
			}
			return base + "/" + suffix, true
		}
		// single id — page resource
		rows, err := s.search.ByID(repoKey, sub)
		if err != nil || len(rows) == 0 {
			return "", false
		}
		ent, ok := s.treeHolder.Get().PageByUUID(rows[0].ID)
		if !ok {
			return "", false
		}
		return "/api/tree/" + ent.Page.TreePath, true
	default:
		return "", false
	}
}

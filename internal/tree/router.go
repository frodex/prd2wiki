package tree

import (
	"net/url"
	"path"
	"strings"
)

// IsReservedRequestPath reports whether p must not be handled by the tree router.
// p should be the request URL path (e.g. r.URL.Path). It is cleaned before checks.
func IsReservedRequestPath(urlPath string) bool {
	p := path.Clean("/" + strings.TrimSpace(urlPath))
	if p == "/" {
		return false
	}
	// Decode once so %2e%2e is visible as ".." after segment split (caller may also reject).
	dec, err := url.PathUnescape(p)
	if err != nil {
		dec = p
	}
	p = path.Clean(dec)
	if p == "/" {
		return false
	}

	trim := strings.TrimPrefix(p, "/")
	first, _, _ := strings.Cut(trim, "/")
	switch first {
	case "api", "static", "blobs", "admin", "projects", "debug":
		return true
	case "health":
		return p == "/health"
	default:
		return false
	}
}

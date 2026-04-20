package api

import (
	"net/http"
	"strings"
)

const scopeWrite = "write"

func apiKeyFromRequest(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if h != "" {
		return h
	}
	return ""
}

// requireWriteScope enforces a valid API key with scope "write" for mutating operations.
func (s *Server) requireWriteScope(w http.ResponseWriter, r *http.Request) bool {
	if s.keys == nil {
		http.Error(w, "API key store not configured", http.StatusInternalServerError)
		return false
	}
	key := apiKeyFromRequest(r)
	if key == "" {
		http.Error(w, "API key required", http.StatusUnauthorized)
		return false
	}
	v, err := s.keys.Validate(r.Context(), key)
	if err != nil {
		http.Error(w, "invalid API key", http.StatusUnauthorized)
		return false
	}
	if _, ok := v.Scopes[scopeWrite]; !ok {
		http.Error(w, "insufficient scope (need write)", http.StatusForbidden)
		return false
	}
	return true
}

package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const principalKey contextKey = "principal"

// RequireAPIKey returns middleware that validates API keys from the Authorization header.
// Format: "Bearer psk_..." or just the raw key.
// Pass requiredScope="" to allow anonymous access (no key needed).
// Pass a non-empty requiredScope to require a valid key with that scope.
func RequireAPIKey(store *ServiceKeyStore, requiredScope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract key from Authorization header.
			authHeader := r.Header.Get("Authorization")
			key := ""
			if strings.HasPrefix(authHeader, "Bearer ") {
				key = strings.TrimPrefix(authHeader, "Bearer ")
			} else if authHeader != "" {
				key = authHeader
			}

			if key == "" {
				// No key — allow anonymous access only when no scope is required.
				if requiredScope == "" {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "API key required", http.StatusUnauthorized)
				return
			}

			// Validate key.
			validation, err := store.Validate(r.Context(), key)
			if err != nil {
				http.Error(w, "invalid API key", http.StatusUnauthorized)
				return
			}

			// Check scope.
			if requiredScope != "" {
				if _, ok := validation.Scopes[requiredScope]; !ok {
					http.Error(w, "insufficient scope", http.StatusForbidden)
					return
				}
			}

			// Set principal in context for downstream handlers.
			ctx := context.WithValue(r.Context(), principalKey, validation.PrincipalID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// PrincipalFromContext retrieves the authenticated principal ID from a request context.
// Returns "" if no principal is set (anonymous request).
func PrincipalFromContext(ctx context.Context) string {
	v, _ := ctx.Value(principalKey).(string)
	return v
}

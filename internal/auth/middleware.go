package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

const (
	// IDTokenCookieName is the name of the ID token cookie
	IDTokenCookieName = "id_token"
)

// AuthMiddleware validates ID token for protected routes (stateless)
// Supports both cookie-based (Web) and header-based (SPA/Mobile) auth
func (c *OIDCClient) AuthMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try to extract ID token from cookie or Authorization header
			idTokenString := c.extractIDToken(r)
			if idTokenString == "" {
				writeUnauthorized(w, "missing authentication token")
				return
			}

			// Verify ID token signature and extract claims (stateless)
			idToken, err := c.VerifyIDToken(r.Context(), idTokenString)
			if err != nil {
				writeUnauthorized(w, "invalid or expired token")
				return
			}

			// Extract user info from token claims
			var userInfo UserInfo
			if err := idToken.Claims(&userInfo); err != nil {
				writeUnauthorized(w, "failed to parse token claims")
				return
			}

			// Add user info to context
			ctx := context.WithValue(r.Context(), UserContextKey, &userInfo)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuthMiddleware extracts user if present but doesn't require it
func (c *OIDCClient) OptionalAuthMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			idTokenString := c.extractIDToken(r)
			if idTokenString != "" {
				if idToken, err := c.VerifyIDToken(r.Context(), idTokenString); err == nil {
					var userInfo UserInfo
					if err := idToken.Claims(&userInfo); err == nil {
						ctx := context.WithValue(r.Context(), UserContextKey, &userInfo)
						r = r.WithContext(ctx)
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// extractIDToken extracts ID token from cookie or Authorization header
func (c *OIDCClient) extractIDToken(r *http.Request) string {
	// 1. Try cookie first (for Web applications)
	if cookie, err := r.Cookie(IDTokenCookieName); err == nil {
		return cookie.Value
	}

	// 2. Try Authorization header (for SPA/Mobile applications)
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		// Support "Bearer <token>" format
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return parts[1]
		}
	}

	return ""
}

func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   "unauthorized",
		"message": message,
	})
}

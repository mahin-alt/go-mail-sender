package api

import (
	"net/http"
	"strings"
)

// WithCORS lets browser apps served from another origin (the separately
// deployed dashboard) call the API.
//
// Concept: browsers block cross-origin requests unless the server opts in via
// Access-Control-* headers, and they send an OPTIONS "preflight" first for
// anything beyond simple requests (JSON bodies, Authorization headers).
// allowedOrigins is an exact-match list; "*" allows any origin. Requests from
// other origins get no CORS headers, so the browser refuses to expose them.
func WithCORS(next http.Handler, allowedOrigins []string) http.Handler {
	allowAll := false
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		switch o {
		case "":
		case "*":
			allowAll = true
		default:
			allowed[o] = true
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowAll || allowed[origin]) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			h.Set("Access-Control-Max-Age", "600")
		}

		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

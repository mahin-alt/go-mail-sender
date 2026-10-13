package api

import "net/http"

// NewRouter maps URLs to handlers - the "table of contents" of the API.
//
// Concept: a router (ServeMux) looks at the method + path of each incoming
// request and picks the handler. Since Go 1.22 the standard library can match
// the HTTP method in the pattern ("POST /path"), so we don't need a
// third-party router. A request with the wrong method automatically gets
// "405 Method Not Allowed", and unknown paths get "404 Not Found".
func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", h.Health)

	// "/api/v1" - the version lives in the URL so we can later ship a /v2
	// with breaking changes without breaking existing clients.
	// "/emails" is a noun (the resource); the HTTP verb POST says "create one".
	mux.HandleFunc("POST /api/v1/emails", h.SendEmail)

	// Excel parsing routes: accept an uploaded .xlsx file and return parsed recipients.
	mux.HandleFunc("POST /api/v1/recipients/parse", h.ParseRecipients)
	mux.HandleFunc("POST /api/v1/recipients/upload", h.ParseRecipients)

	return mux
}

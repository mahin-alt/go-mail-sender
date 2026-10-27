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

	// Single email delivery (synchronous)
	mux.HandleFunc("POST /api/v1/emails", h.SendEmail)

	// Excel parsing routes: accept an uploaded .xlsx file and return parsed recipients.
	mux.HandleFunc("POST /api/v1/recipients/parse", h.ParseRecipients)

	// Background batch job routes: upload Excel + template, queue jobs in PostgreSQL,
	// and query asynchronous progress.
	mux.HandleFunc("POST /api/v1/batches", h.CreateBatch)
	mux.HandleFunc("GET /api/v1/batches", h.ListBatches)
	mux.HandleFunc("GET /api/v1/batches/{id}", h.GetBatch)
	mux.HandleFunc("GET /api/v1/batches/{id}/jobs", h.GetBatchJobs)
	mux.HandleFunc("POST /api/v1/batches/{id}/retry", h.RetryBatch)
	mux.HandleFunc("POST /api/v1/jobs/{id}/retry", h.RetryJob)

	return mux
}

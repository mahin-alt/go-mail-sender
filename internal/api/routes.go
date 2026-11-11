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

	// Verify Emails: upload a .xlsx to check it (202, imported in the
	// background), poll its progress, page through the rows that will not be sent.
	mux.HandleFunc("POST /api/v1/verify", h.CreateVerification)
	mux.HandleFunc("GET /api/v1/verify/{id}", h.GetBatch)
	mux.HandleFunc("GET /api/v1/verify/{id}/issues", h.ListIssues)

	// Bulk Emails: upload, review, explicitly send, follow delivery, cancel, retry.
	// A verified batch is sent with the same /batches/{id}/send.
	mux.HandleFunc("POST /api/v1/batches", h.CreateBatch)
	mux.HandleFunc("GET /api/v1/batches", h.ListBatches)
	mux.HandleFunc("GET /api/v1/batches/{id}", h.GetBatch)
	mux.HandleFunc("GET /api/v1/batches/{id}/issues", h.ListIssues)
	mux.HandleFunc("GET /api/v1/batches/{id}/sample", h.GetSample)
	mux.HandleFunc("POST /api/v1/batches/{id}/send", h.SendBatch)
	mux.HandleFunc("GET /api/v1/batches/{id}/emails", h.ListEmails)
	mux.HandleFunc("POST /api/v1/batches/{id}/cancel", h.CancelBatch)
	mux.HandleFunc("POST /api/v1/batches/{id}/retry", h.RetryBatch)
	mux.HandleFunc("POST /api/v1/jobs/{id}/retry", h.RetryJob)

	// Dashboard Overview numbers.
	mux.HandleFunc("GET /api/v1/stats", h.GetStats)

	// Settings: view and edit runtime configuration, and test SMTP.
	mux.HandleFunc("GET /api/v1/settings", h.GetSettings)
	mux.HandleFunc("PUT /api/v1/settings", h.UpdateSettings)
	mux.HandleFunc("POST /api/v1/settings/smtp/test", h.TestSMTP)

	return mux
}

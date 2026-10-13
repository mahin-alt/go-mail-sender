// Package api is the HTTP layer: it translates between the outside world
// (JSON over HTTP) and our internal code (the Mailer).
//
// Its job, in order, for every request:
//  1. decode  - turn the JSON body into a Go struct
//  2. validate - never trust input from the network
//  3. act     - call the Mailer
//  4. respond - write a JSON response with the right status code
package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"strings"

	"sendmail/internal/mailer"
)

// maxBodyBytes caps how much of a request body we will read, so a client
// can't exhaust our memory by streaming gigabytes at us.
const maxBodyBytes = 1 << 20 // 1 MB

// Handler holds the dependencies our HTTP handlers need.
//
// Concept: dependency injection, the Go way - no framework, just a struct
// with fields, filled in by main(). Handlers are methods on it so they can
// reach h.mailer. Because the field is an INTERFACE, tests can inject a fake.
type Handler struct {
	mailer mailer.Mailer
}

// NewHandler builds a Handler.
func NewHandler(m mailer.Mailer) *Handler {
	return &Handler{mailer: m}
}

// sendEmailRequest is the JSON shape clients send. It is the "request" half
// of our API contract (see README). The `json:"..."` tags map JSON keys to
// struct fields.
type sendEmailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Health reports that the process is up. Load balancers and orchestrators
// poll an endpoint like this. It deliberately doesn't touch SMTP.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// SendEmail handles POST /api/v1/emails.
func (h *Handler) SendEmail(w http.ResponseWriter, r *http.Request) {
	// 1. DECODE ---------------------------------------------------------
	// MaxBytesReader makes reads fail once the limit is exceeded.
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	// Reject JSON keys we don't know about: typos like "subjet" become an
	// immediate, visible error instead of a silently empty subject.
	dec.DisallowUnknownFields()

	var req sendEmailRequest
	if err := dec.Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// 2. VALIDATE -------------------------------------------------------
	// A 4xx status means "the CLIENT made a mistake, fix your request".
	if msg := validate(req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	// 3. ACT ------------------------------------------------------------
	if err := h.mailer.Send(req.To, req.Subject, req.Body); err != nil {
		// The real error (may contain hostnames, auth details...) goes to
		// OUR logs only. The client gets a generic message.
		// 502 Bad Gateway = "we are fine, the upstream (SMTP) failed".
		// A 5xx means "the SERVER side had a problem, not your request".
		log.Printf("send email failed: %v", err)
		writeError(w, http.StatusBadGateway, "failed to send email")
		return
	}

	// 4. RESPOND --------------------------------------------------------
	// We send synchronously, so by now the mail was accepted by the SMTP
	// server: 200 OK. (If we queued it for later we'd return 202 Accepted.)
	writeJSON(w, http.StatusOK, map[string]string{"message": "email sent"})
}

// validate returns a human-readable problem, or "" if the request is fine.
func validate(req sendEmailRequest) string {
	if strings.TrimSpace(req.To) == "" {
		return "\"to\" is required"
	}
	// ParseAddress checks the address syntax. It also accepts "Name <a@b.com>",
	// which is fine for us.
	if _, err := mail.ParseAddress(req.To); err != nil {
		return "\"to\" is not a valid email address"
	}
	if strings.TrimSpace(req.Subject) == "" {
		return "\"subject\" is required"
	}
	if strings.TrimSpace(req.Body) == "" {
		return "\"body\" is required"
	}
	// Header injection defence: CR/LF in a header value would let an
	// attacker add their own headers. See the note in mailer.Send.
	if strings.ContainsAny(req.To, "\r\n") || strings.ContainsAny(req.Subject, "\r\n") {
		return "\"to\" and \"subject\" must not contain line breaks"
	}
	return ""
}

// writeJSON writes v as JSON with the given status code.
// Order matters: headers first, then WriteHeader (status), then the body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response failed: %v", err)
	}
}

// writeError keeps every error response the same shape: {"error": "..."}.
// A consistent shape means clients need only one piece of error-handling code.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

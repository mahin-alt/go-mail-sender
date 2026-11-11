// Package api is the HTTP layer: it translates between the outside world
// (JSON over HTTP) and our internal packages (Store, Mailer, Worker).
//
// Its job, in order, for every request:
//  1. decode   - turn the request body / multipart form into Go data
//  2. validate - never trust input from the network
//  3. act      - call the Store / Mailer / Worker
//  4. respond  - write a JSON response with the right status code
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"sendmail/internal/config"
	"sendmail/internal/importer"
	"sendmail/internal/mailer"
	"sendmail/internal/settings"
	"sendmail/internal/store"
)

// Waker is a background worker the API can nudge after creating work for it
// (an upload to import, a batch to generate, emails to send), so it starts
// now instead of at its next poll.
type Waker interface{ Wake() }

// Handler holds the dependencies our HTTP handlers need.
//
// Concept: dependency injection. Handlers receive dependencies via struct fields
// rather than global variables. This allows tests to pass fakes/mocks easily.
type Handler struct {
	mailer mailer.Mailer
	store  *store.Store

	// Background workers to wake; nil in tests that do not need them.
	sender    Waker
	importer  Waker
	generator Waker

	files importer.Files // where uploaded spreadsheets are saved

	// maxUploadMB is the largest accepted spreadsheet, changed from the
	// Settings page while requests are reading it, hence atomic.
	maxUploadMB atomic.Int64

	settings *settings.Service // nil = settings endpoints disabled
}

// NewHandler builds a Handler. store and sender can be nil in isolated tests
// that only exercise validation.
func NewHandler(m mailer.Mailer, st *store.Store, sender Waker) *Handler {
	h := &Handler{
		mailer: m,
		store:  st,
		sender: sender,
		files:  importer.Files{Dir: filepath.Join(os.TempDir(), "go-mail-sender-uploads")},
	}
	h.maxUploadMB.Store(config.DefaultMaxUploadMB)
	return h
}

// WithImports sets where uploads are saved and the import and job-generation
// workers to wake, and returns h for chaining.
func (h *Handler) WithImports(files importer.Files, imports, generator Waker) *Handler {
	h.files, h.importer, h.generator = files, imports, generator
	return h
}

// SetMaxUploadMB changes the upload size limit at runtime. Uploads already in
// progress keep the limit they started with. Values below 1 are ignored.
func (h *Handler) SetMaxUploadMB(mb int) {
	if mb > 0 {
		h.maxUploadMB.Store(int64(mb))
	}
}

// WithSettings enables the /api/v1/settings endpoints.
func (h *Handler) WithSettings(s *settings.Service) *Handler {
	h.settings = s
	return h
}

func wake(w Waker) {
	if w != nil {
		w.Wake()
	}
}

// sendEmailRequest is the JSON shape clients send to POST /api/v1/emails.
type sendEmailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Health reports that the process is up.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// SendEmail handles POST /api/v1/emails (synchronous single email).
func (h *Handler) SendEmail(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
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

	if msg := validateEmailReq(req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	if err := h.mailer.Send(req.To, req.Subject, req.Body); err != nil {
		slog.Error("send email failed", "err", err)
		writeError(w, http.StatusBadGateway, "failed to send email")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "email sent"})
}

func validateEmailReq(req sendEmailRequest) string {
	if strings.TrimSpace(req.To) == "" {
		return "\"to\" is required"
	}
	if _, err := mail.ParseAddress(req.To); err != nil {
		return "\"to\" is not a valid email address"
	}
	if strings.TrimSpace(req.Subject) == "" {
		return "\"subject\" is required"
	}
	if strings.TrimSpace(req.Body) == "" {
		return "\"body\" is required"
	}
	if strings.ContainsAny(req.To, "\r\n") || strings.ContainsAny(req.Subject, "\r\n") {
		return "\"to\" and \"subject\" must not contain line breaks"
	}
	return ""
}

// ms rounds a duration to milliseconds for logging ("1.081s", "12ms").
func ms(d time.Duration) time.Duration { return d.Round(time.Millisecond) }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("write response failed", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

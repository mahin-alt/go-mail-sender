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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"slices"
	"strconv"
	"strings"

	"sendmail/internal/domaincheck"
	"sendmail/internal/mailer"
	"sendmail/internal/recipients"
	"sendmail/internal/store"
	"sendmail/internal/template"
	"sendmail/internal/worker"
)

// Limits on request bodies to prevent resource exhaustion.
const (
	maxBodyBytes   = 1 << 20  // 1 MB for JSON payloads
	maxUploadBytes = 10 << 20 // 10 MB for file uploads
)

// Handler holds the dependencies our HTTP handlers need.
//
// Concept: dependency injection. Handlers receive dependencies via struct fields
// rather than global variables. This allows tests to pass fakes/mocks easily.
type Handler struct {
	mailer  mailer.Mailer
	store   *store.Store
	worker  *worker.Pool
	domains DomainChecker // nil = domain (DNS/MX) checking disabled
}

// DomainChecker reports whether an email domain can receive mail.
// *domaincheck.Checker satisfies it; tests pass a fake.
type DomainChecker interface {
	Check(ctx context.Context, domain string) domaincheck.Result
}

// WithDomainChecker enables the recipient domain check and returns h for chaining.
func (h *Handler) WithDomainChecker(c DomainChecker) *Handler {
	h.domains = c
	return h
}

// verifyDomains runs the domain check over parsed recipients.
//   - ok:         recipients to keep (domain valid, or could not be verified)
//   - rejected:   rows whose domain definitely cannot receive mail
//   - unverified: kept rows whose lookup failed (e.g. DNS timeout), reported so
//     "confirmed" and "could not check" are distinguishable
//
// A DNS failure never rejects a row: wrongly dropping a real customer costs
// more than a bounce. With no checker configured everything is kept as-is.
func (h *Handler) verifyDomains(ctx context.Context, recs []recipients.Recipient) (ok []recipients.Recipient, rejected, unverified []recipients.RowError) {
	if h.domains == nil {
		return recs, nil, nil
	}

	// Look each domain up once per request (the rows are checked one after
	// another; a shared cache and concurrency come in a later phase).
	results := make(map[string]domaincheck.Result)

	ok = make([]recipients.Recipient, 0, len(recs))
	for _, rec := range recs {
		domain := rec.Email[strings.LastIndex(rec.Email, "@")+1:]
		res, seen := results[domain]
		if !seen {
			res = h.domains.Check(ctx, domain)
			results[domain] = res
		}

		switch res.Status {
		case domaincheck.Invalid:
			rejected = append(rejected, recipients.RowError{Row: rec.Row, Name: rec.Name, Email: rec.Email, Reason: res.Reason})
		case domaincheck.Unknown:
			unverified = append(unverified, recipients.RowError{Row: rec.Row, Name: rec.Name, Email: rec.Email, Reason: res.Reason})
			ok = append(ok, rec)
		default:
			ok = append(ok, rec)
		}
	}

	if len(unverified) > 0 {
		log.Printf("domain check: %d recipient(s) kept but could not be verified (DNS errors)", len(unverified))
	}
	return ok, rejected, unverified
}

// mergeProblems appends rejected rows to problems, keeping spreadsheet row order.
func mergeProblems(problems, rejected []recipients.RowError) []recipients.RowError {
	if len(rejected) == 0 {
		return problems
	}
	problems = append(problems, rejected...)
	slices.SortStableFunc(problems, func(a, b recipients.RowError) int { return a.Row - b.Row })
	return problems
}

// NewHandler builds a Handler with all required dependencies.
// store and worker can be nil in isolated tests that only test synchronous endpoints.
func NewHandler(m mailer.Mailer, st *store.Store, wp *worker.Pool) *Handler {
	return &Handler{
		mailer: m,
		store:  st,
		worker: wp,
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
		log.Printf("send email failed: %v", err)
		writeError(w, http.StatusBadGateway, "failed to send email")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "email sent"})
}

// parseRecipientsResponse is the response shape for POST /api/v1/recipients/parse.
type parseRecipientsResponse struct {
	Valid           []recipients.Recipient `json:"valid"`
	Problems        []recipients.RowError  `json:"problems"`
	UnverifiedCount int                    `json:"unverified_count"`
	Unverified      []recipients.RowError  `json:"unverified,omitempty"`
}

// ParseRecipients handles POST /api/v1/recipients/parse (preview parsed Excel recipients).
func (h *Handler) ParseRecipients(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	if err := r.ParseMultipartForm(5 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, `form field "file" is required`)
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".xlsx") {
		writeError(w, http.StatusBadRequest, "only .xlsx files are supported")
		return
	}

	valid, problems, err := recipients.ParseExcelReader(file)
	if err != nil {
		if errors.Is(err, recipients.ErrMissingColumns) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse excel file: %v", err))
		return
	}

	valid, rejected, unverified := h.verifyDomains(r.Context(), valid)
	problems = mergeProblems(problems, rejected)

	writeJSON(w, http.StatusOK, parseRecipientsResponse{
		Valid:           valid,
		Problems:        problems,
		UnverifiedCount: len(unverified),
		Unverified:      unverified,
	})
}

// createBatchResponse is returned by POST /api/v1/batches.
type createBatchResponse struct {
	BatchID      int64                 `json:"batch_id"`
	Filename     string                `json:"filename"`
	TotalRows    int                   `json:"total_rows"`
	ValidCount   int                   `json:"valid_count"`
	InvalidCount int                   `json:"invalid_count"`
	Status       string                `json:"status"`
	Problems     []recipients.RowError `json:"problems,omitempty"`
	// UnverifiedCount counts valid rows whose domain lookup failed (e.g. DNS
	// timeout). They are still queued; see Unverified for which ones.
	UnverifiedCount int                   `json:"unverified_count"`
	Unverified      []recipients.RowError `json:"unverified,omitempty"`
	Message         string                `json:"message"`
}

// CreateBatch handles POST /api/v1/batches:
// Accepts an uploaded Excel file + optional subject & body templates,
// creates a Batch in PostgreSQL, renders personalized email bodies for each recipient,
// bulk-inserts the jobs, and signals background workers to process them.
func (h *Handler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusInternalServerError, "database store is not configured")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	if err := r.ParseMultipartForm(5 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "uploaded file too large (max 10MB)")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, `form field "file" is required`)
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".xlsx") {
		writeError(w, http.StatusBadRequest, "only .xlsx files are supported")
		return
	}

	// Read optional template overrides from the form.
	// Defaults provide an out-of-the-box personalized greeting using {{.Name}}.
	subjectTmpl := strings.TrimSpace(r.FormValue("subject"))
	if subjectTmpl == "" {
		subjectTmpl = "Hello {{.Name}}"
	}

	bodyTmpl := strings.TrimSpace(r.FormValue("body"))
	if bodyTmpl == "" {
		bodyTmpl = "Hi {{.Name}},\n\nWelcome to our platform!"
	}

	// Validate template syntax before creating any database records
	if err := template.Validate(subjectTmpl); err != nil {
		writeError(w, http.StatusBadRequest, "invalid subject template: "+err.Error())
		return
	}
	if err := template.Validate(bodyTmpl); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body template: "+err.Error())
		return
	}

	// Parse Excel recipients
	validRecipients, problems, err := recipients.ParseExcelReader(file)
	if err != nil {
		if errors.Is(err, recipients.ErrMissingColumns) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse excel file: %v", err))
		return
	}

	if len(validRecipients) == 0 && len(problems) == 0 {
		writeError(w, http.StatusBadRequest, "no recipient rows found in the uploaded excel file")
		return
	}

	// Reject rows whose domain cannot receive mail; they are recorded below as
	// 'invalid' jobs together with the syntax problems.
	validRecipients, rejected, unverified := h.verifyDomains(r.Context(), validRecipients)
	problems = mergeProblems(problems, rejected)

	jobs := make([]store.Job, 0, len(validRecipients)+len(problems))

	// 1. Build jobs for valid recipients (status = 'pending')
	for _, rec := range validRecipients {
		data := template.RecipientData{
			Name:  rec.Name,
			Email: rec.Email,
		}

		renderedSubject, err := template.Render(subjectTmpl, data)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("render subject for %s: %v", rec.Email, err))
			return
		}

		renderedBody, err := template.Render(bodyTmpl, data)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("render body for %s: %v", rec.Email, err))
			return
		}

		jobs = append(jobs, store.Job{
			RecipientName:  rec.Name,
			RecipientEmail: rec.Email,
			Subject:        renderedSubject,
			Body:           renderedBody,
			Status:         "pending",
		})
	}

	// 2. Build jobs for invalid recipients (status = 'invalid') so they are tracked
	// in the database without being picked up by background workers.
	for _, prob := range problems {
		reason := prob.Reason
		jobs = append(jobs, store.Job{
			RecipientName:  prob.Name,
			RecipientEmail: prob.Email,
			Subject:        "",
			Body:           "",
			Status:         "invalid",
			LastError:      &reason,
		})
	}

	// Save batch and all rows (valid + invalid) atomically inside a database transaction
	batchID, err := h.store.CreateBatchWithJobs(r.Context(), header.Filename, subjectTmpl, bodyTmpl, jobs)
	if err != nil {
		log.Printf("create batch failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create batch in database")
		return
	}

	// Wake up idle workers only if there are pending jobs to send
	if len(validRecipients) > 0 && h.worker != nil {
		h.worker.Wake()
	}

	batchStatus := "pending"
	if len(validRecipients) == 0 {
		batchStatus = "completed"
	}

	// 202 Accepted: request is accepted
	writeJSON(w, http.StatusAccepted, createBatchResponse{
		BatchID:         batchID,
		Filename:        header.Filename,
		TotalRows:       len(jobs),
		ValidCount:      len(validRecipients),
		InvalidCount:    len(problems),
		Status:          batchStatus,
		Problems:        problems,
		UnverifiedCount: len(unverified),
		Unverified:      unverified,
		Message:         "batch accepted; valid emails queued for delivery and invalid rows recorded",
	})
}

// GetBatch handles GET /api/v1/batches/{id}:
// Returns progress and status metrics for a batch.
func (h *Handler) GetBatch(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusInternalServerError, "database store is not configured")
		return
	}

	idStr := r.PathValue("id")
	batchID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || batchID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid batch id")
		return
	}

	progress, err := h.store.GetBatchProgress(r.Context(), batchID)
	if err != nil {
		if errors.Is(err, store.ErrBatchNotFound) {
			writeError(w, http.StatusNotFound, "batch not found")
			return
		}
		log.Printf("get batch progress failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get batch progress")
		return
	}

	writeJSON(w, http.StatusOK, progress)
}

// GetBatchJobs handles GET /api/v1/batches/{id}/jobs:
// Lists jobs for a batch, optionally filtered by ?status=invalid (or pending/sent/failed).
func (h *Handler) GetBatchJobs(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusInternalServerError, "database store is not configured")
		return
	}

	idStr := r.PathValue("id")
	batchID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || batchID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid batch id")
		return
	}

	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	jobs, err := h.store.GetBatchJobs(r.Context(), batchID, statusFilter)
	if err != nil {
		log.Printf("get batch jobs failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch batch jobs")
		return
	}

	if jobs == nil {
		jobs = []store.Job{}
	}

	writeJSON(w, http.StatusOK, jobs)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response failed: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// batchListResponse is the paged envelope returned by ListBatches.
type batchListResponse struct {
	Items  []store.BatchSummary `json:"items"`
	Total  int                  `json:"total"`
	Limit  int                  `json:"limit"`
	Offset int                  `json:"offset"`
}

// Paging bounds for list endpoints.
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// ListBatches handles GET /api/v1/batches:
// Lists batches newest-first with job counts. Query params: status, q
// (filename search), limit (default 20, max 100) and offset.
func (h *Handler) ListBatches(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusInternalServerError, "database store is not configured")
		return
	}

	q := r.URL.Query()
	limit, offset := defaultPageSize, 0
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = min(n, maxPageSize)
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid offset")
			return
		}
		offset = n
	}

	items, total, err := h.store.ListBatches(r.Context(), store.BatchListFilter{
		Status: strings.TrimSpace(q.Get("status")),
		Search: strings.TrimSpace(q.Get("q")),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		log.Printf("list batches failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list batches")
		return
	}

	writeJSON(w, http.StatusOK, batchListResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

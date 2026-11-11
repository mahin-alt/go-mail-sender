package api

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"sendmail/internal/importer"
	"sendmail/internal/store"
	"sendmail/internal/template"
)

// Limits for request bodies and pages.
const (
	maxBodyBytes = 1 << 20 // 1 MB for JSON payloads

	// formOverheadBytes is room for the multipart boundaries around the file,
	// so a file exactly at the size limit is still accepted.
	formOverheadBytes = 1 << 20

	defaultPageSize   = 20
	maxPageSize       = 100
	defaultIssuePage  = 100
	maxIssuePage      = 500
	defaultSampleSize = 5
	maxSampleSize     = 20
	maxSearchLen      = 200
	maxSubjectLen     = 998 // the longest header line SMTP allows
	maxTemplateLen    = 100_000
)

// ---- uploads -----------------------------------------------------------

// uploadResponse is returned by POST /verify and POST /batches: the import
// continues in the background, poll GET .../{id} for progress.
type uploadResponse struct {
	BatchID  int64  `json:"batch_id"`
	Type     string `json:"type"`
	Filename string `json:"filename"`
	Status   string `json:"status"`
}

// CreateVerification handles POST /api/v1/verify: upload a .xlsx to check it.
func (h *Handler) CreateVerification(w http.ResponseWriter, r *http.Request) {
	h.upload(w, r, store.TypeVerification)
}

// CreateBatch handles POST /api/v1/batches: upload a .xlsx to send to.
func (h *Handler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	h.upload(w, r, store.TypeBulkSend)
}

// upload streams the "file" field of a multipart form straight to disk (never
// the whole file in memory), creates the batch and its import task, and
// answers 202 at once. The file is read and checked by the import worker.
func (h *Handler) upload(w http.ResponseWriter, r *http.Request, batchType string) {
	start := time.Now()
	limitMB := h.maxUploadMB.Load() // read once: a settings change mid-upload does not affect it
	limit := limitMB << 20
	tooLarge := fmt.Sprintf("file too large: the limit is %d MB", limitMB)
	r.Body = http.MaxBytesReader(w, r.Body, limit+formOverheadBytes)

	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "expected a multipart form with a \"file\" field")
		return
	}
	var tmp, filename string
	for tmp == "" {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if isTooBig(err) {
				writeError(w, http.StatusRequestEntityTooLarge, tooLarge)
			} else {
				writeError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
			}
			return
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		filename = cleanFilename(part.FileName())
		if !strings.HasSuffix(strings.ToLower(filename), ".xlsx") {
			part.Close()
			writeError(w, http.StatusBadRequest, "only .xlsx files are supported")
			return
		}
		tmp, err = h.files.Receive(part, limit)
		part.Close()
		switch {
		case err == nil:
		case errors.Is(err, importer.ErrTooLarge) || isTooBig(err):
			writeError(w, http.StatusRequestEntityTooLarge, tooLarge)
			return
		case errors.Is(err, importer.ErrNotXLSX):
			writeError(w, http.StatusBadRequest, err.Error())
			return
		default:
			slog.Error("save upload failed", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to save the uploaded file")
			return
		}
	}
	if tmp == "" {
		writeError(w, http.StatusBadRequest, `form field "file" is required`)
		return
	}
	// If anything below fails, do not leave the file behind. (After a
	// successful Promote the temporary name no longer exists; Remove is a no-op.)
	defer os.Remove(tmp)

	if h.store == nil {
		writeError(w, http.StatusInternalServerError, "database store is not configured")
		return
	}
	id, err := h.store.CreateBatch(r.Context(), batchType, filename, func(id int64) error {
		return h.files.Promote(tmp, id)
	})
	if err != nil {
		slog.Error("create batch failed", "filename", filename, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to create the batch")
		return
	}
	wake(h.importer)
	slog.Info("upload received", "batch_id", id, "type", batchType, "filename", filename, "took", ms(time.Since(start)))
	writeJSON(w, http.StatusAccepted, uploadResponse{BatchID: id, Type: batchType, Filename: filename, Status: store.StatusImporting})
}

func isTooBig(err error) bool {
	var tooBig *http.MaxBytesError
	return errors.As(err, &tooBig)
}

// cleanFilename keeps only the last path element of a client filename (it is
// shown in the UI, never used as a path) and caps its length.
func cleanFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	if name == "." || name == "/" {
		return ""
	}
	if r := []rune(name); len(r) > 255 {
		name = string(r[len(r)-255:]) // keep the end, where ".xlsx" is
	}
	return name
}

// ---- reading batches -----------------------------------------------------

// batchID reads {id} from the path; on a bad value it writes 400 and returns 0.
func (h *Handler) batchID(w http.ResponseWriter, r *http.Request) int64 {
	if h.store == nil {
		writeError(w, http.StatusInternalServerError, "database store is not configured")
		return 0
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid batch id")
		return 0
	}
	return id
}

// loadBatch fetches {id}, writing 404 / 500 itself when that fails.
func (h *Handler) loadBatch(w http.ResponseWriter, r *http.Request) *store.Batch {
	id := h.batchID(w, r)
	if id == 0 {
		return nil
	}
	b, err := h.store.GetBatch(r.Context(), id)
	if errors.Is(err, store.ErrBatchNotFound) {
		writeError(w, http.StatusNotFound, "batch not found")
		return nil
	}
	if err != nil {
		slog.Error("get batch failed", "batch_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch the batch")
		return nil
	}
	return b
}

// GetBatch handles GET /api/v1/verify/{id} and GET /api/v1/batches/{id}: the
// batch's status, counters and import / delivery progress. Small and cheap,
// so pages can poll it every few seconds.
func (h *Handler) GetBatch(w http.ResponseWriter, r *http.Request) {
	if b := h.loadBatch(w, r); b != nil {
		writeJSON(w, http.StatusOK, b)
	}
}

// batchListResponse is the paged envelope returned by ListBatches.
type batchListResponse struct {
	Items  []store.Batch `json:"items"`
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

var (
	batchTypes    = []string{store.TypeVerification, store.TypeBulkSend}
	batchStatuses = []string{store.StatusImporting, store.StatusImportFailed, store.StatusDraft,
		store.StatusGeneratingJobs, store.StatusQueued, store.StatusSending, store.StatusCompleted,
		store.StatusCompletedWithErrors, store.StatusFailed, store.StatusCancelled}
	jobStatuses = []string{store.JobPending, store.JobProcessing, store.JobSent, store.JobFailed, store.JobCancelled}
)

// ListBatches handles GET /api/v1/batches: newest first, filtered by ?type=,
// ?status= and ?q= (filename), paged with ?limit= (default 20, max 100) and
// ?offset=.
func (h *Handler) ListBatches(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusInternalServerError, "database store is not configured")
		return
	}
	q := r.URL.Query()
	typ, ok := oneOf(w, q, "type", batchTypes)
	if !ok {
		return
	}
	status, ok := oneOf(w, q, "status", batchStatuses)
	if !ok {
		return
	}
	search, ok := searchParam(w, q)
	if !ok {
		return
	}
	limit, ok := intParam(w, q.Get("limit"), defaultPageSize, 1, "limit")
	if !ok {
		return
	}
	limit = min(limit, maxPageSize)
	offset, ok := intParam(w, q.Get("offset"), 0, 0, "offset")
	if !ok {
		return
	}

	items, total, err := h.store.ListBatches(r.Context(), store.BatchListFilter{
		Type: typ, Status: status, Search: search, Limit: limit, Offset: offset,
	})
	if err != nil {
		slog.Error("list batches failed", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list batches")
		return
	}
	writeJSON(w, http.StatusOK, batchListResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

// cursorPage is one page of rows. NextAfter is the value to send as ?after=
// for the next page, or null when this was the last page.
type cursorPage[T any] struct {
	Items     []T    `json:"items"`
	NextAfter *int64 `json:"next_after"`
}

// ListIssues handles GET /api/v1/verify/{id}/issues and
// GET /api/v1/batches/{id}/issues: the rows that will not be sent, or the
// ones whose domain could not be checked.
//
//	?status=invalid (default) | unverified
//	?reason=bad_domain         invalid rows with these reasons (comma separated)
//	?q=aisha                   search in name and email
//	?after=<id>&limit=100      the page (cursor from next_after)
func (h *Handler) ListIssues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := store.RecipientInvalid
	if v := q.Get("status"); v != "" {
		if v != store.RecipientInvalid && v != store.RecipientUnverified {
			writeError(w, http.StatusBadRequest, "invalid status (allowed: invalid, unverified)")
			return
		}
		status = v
	}
	var reasons []string
	if v := strings.TrimSpace(q.Get("reason")); v != "" {
		for _, reason := range strings.Split(v, ",") {
			reason = strings.TrimSpace(reason)
			if !slices.Contains(store.InvalidReasons, reason) {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid reason %q (allowed: %s)", reason, strings.Join(store.InvalidReasons, ", ")))
				return
			}
			reasons = append(reasons, reason)
		}
	}
	if len(reasons) > 0 && status != store.RecipientInvalid {
		writeError(w, http.StatusBadRequest, "reason only applies to status=invalid")
		return
	}
	search, ok := searchParam(w, q)
	if !ok {
		return
	}
	after, ok := cursorParam(w, q)
	if !ok {
		return
	}
	limit, ok := intParam(w, q.Get("limit"), defaultIssuePage, 1, "limit")
	if !ok {
		return
	}
	limit = min(limit, maxIssuePage)

	b := h.loadBatch(w, r)
	if b == nil {
		return
	}
	// Ask for one extra row: if it comes back, there is a next page.
	rows, err := h.store.ListRecipients(r.Context(), b.ID, store.RecipientFilter{
		Status: status, Reasons: reasons, Search: search, After: after, Limit: limit + 1,
	})
	if err != nil {
		slog.Error("list issues failed", "batch_id", b.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch rows")
		return
	}
	page := cursorPage[store.Recipient]{Items: rows}
	if len(rows) > limit {
		page.Items = rows[:limit]
		next := page.Items[limit-1].ID
		page.NextAfter = &next
	}
	writeJSON(w, http.StatusOK, page)
}

// GetSample handles GET /api/v1/batches/{id}/sample?limit=5: the first few
// rows that will be sent, so the New Bulk Email preview can show real names.
func (h *Handler) GetSample(w http.ResponseWriter, r *http.Request) {
	limit, ok := intParam(w, r.URL.Query().Get("limit"), defaultSampleSize, 1, "limit")
	if !ok {
		return
	}
	limit = min(limit, maxSampleSize)
	b := h.loadBatch(w, r)
	if b == nil {
		return
	}
	rows, err := h.store.ListRecipients(r.Context(), b.ID, store.RecipientFilter{Status: store.Sendable, Limit: limit})
	if err != nil {
		slog.Error("sample failed", "batch_id", b.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch sample rows")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

// emailView is one email in the delivery list, with its subject and body
// rendered from the batch templates (they are not stored per email).
type emailView struct {
	store.EmailItem
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// ListEmails handles GET /api/v1/batches/{id}/emails: the batch's emails and
// their delivery state, oldest first.
//
//	?status=failed          pending | processing | sent | failed | cancelled
//	?q=aisha                search in name and email
//	?after=<id>&limit=20    the page (cursor from next_after)
func (h *Handler) ListEmails(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status, ok := oneOf(w, q, "status", jobStatuses)
	if !ok {
		return
	}
	search, ok := searchParam(w, q)
	if !ok {
		return
	}
	after, ok := cursorParam(w, q)
	if !ok {
		return
	}
	limit, ok := intParam(w, q.Get("limit"), defaultPageSize, 1, "limit")
	if !ok {
		return
	}
	limit = min(limit, maxPageSize)

	b := h.loadBatch(w, r)
	if b == nil {
		return
	}
	items, err := h.store.ListBatchJobs(r.Context(), b.ID, store.JobListFilter{
		Status: status, Search: search, After: after, Limit: limit + 1,
	})
	if err != nil {
		slog.Error("list emails failed", "batch_id", b.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch emails")
		return
	}
	page := cursorPage[emailView]{Items: make([]emailView, 0, len(items))}
	for _, it := range items[:min(len(items), limit)] {
		data := template.RecipientData{Name: it.Name, Email: it.Email}
		subject, _ := template.Render(b.SubjectTemplate, data)
		body, _ := template.Render(b.BodyTemplate, data)
		page.Items = append(page.Items, emailView{EmailItem: it, Subject: oneLine(subject), Body: body})
	}
	if len(items) > limit {
		next := items[limit-1].ID
		page.NextAfter = &next
	}
	writeJSON(w, http.StatusOK, page)
}

// oneLine replaces line breaks with spaces: a subject is one header line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")), " ")
}

// ---- actions ---------------------------------------------------------------

type sendRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// SendBatch handles POST /api/v1/batches/{id}/send: the explicit "Send" for a
// draft bulk batch, or for a checked verification batch (verify first, send
// later). It saves the templates and starts job generation; emails are built
// from the templates at send time. 409 if the batch cannot be sent (still
// importing, already sending, cancelled...) or has no one to send to.
func (h *Handler) SendBatch(w http.ResponseWriter, r *http.Request) {
	id := h.batchID(w, r)
	if id == 0 {
		return
	}
	var req sendRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Subject, req.Body = strings.TrimSpace(req.Subject), strings.TrimSpace(req.Body)
	if msg := validateTemplates(req.Subject, req.Body); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	err := h.store.RequestSend(r.Context(), id, req.Subject, req.Body)
	switch {
	case errors.Is(err, store.ErrBatchNotFound):
		writeError(w, http.StatusNotFound, "batch not found")
		return
	case errors.Is(err, store.ErrNotSendable):
		writeError(w, http.StatusConflict, "this batch cannot be sent: it is still importing, failed to import, or was already sent")
		return
	case errors.Is(err, store.ErrNothingToSend):
		writeError(w, http.StatusConflict, "this batch has no valid recipients to send to")
		return
	case err != nil:
		slog.Error("request send failed", "batch_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to start sending")
		return
	}
	wake(h.generator)
	slog.Info("send requested", "batch_id", id)
	if b := h.loadBatch(w, r); b != nil {
		writeJSON(w, http.StatusAccepted, b)
	}
}

// validateTemplates checks the subject and body before anything is saved:
// required, not too long, valid template syntax, and only the fields a
// recipient has ({{.Name}}, {{.Email}}). Each is rendered once with a test
// recipient, because an unknown field such as {{.Phone}} only fails when the
// template runs, and that would otherwise fail every single email.
func validateTemplates(subject, body string) string {
	switch {
	case subject == "":
		return `"subject" is required`
	case body == "":
		return `"body" is required`
	case len(subject) > maxSubjectLen:
		return fmt.Sprintf(`"subject" is too long (max %d characters)`, maxSubjectLen)
	case len(body) > maxTemplateLen:
		return fmt.Sprintf(`"body" is too long (max %d characters)`, maxTemplateLen)
	case strings.ContainsAny(subject, "\r\n"):
		return `"subject" must be one line`
	}
	test := template.RecipientData{Name: "Test Person", Email: "test@example.com"}
	if _, err := template.Render(subject, test); err != nil {
		return "invalid subject template: " + err.Error()
	}
	if _, err := template.Render(body, test); err != nil {
		return "invalid body template: " + err.Error()
	}
	return ""
}

type cancelResponse struct {
	Cancelled int `json:"cancelled"`
}

// CancelBatch handles POST /api/v1/batches/{id}/cancel: stops a batch that is
// generating or sending. Unsent emails become 'cancelled' and are never sent
// or retried; an email being sent right now finishes.
func (h *Handler) CancelBatch(w http.ResponseWriter, r *http.Request) {
	id := h.batchID(w, r)
	if id == 0 {
		return
	}
	n, err := h.store.CancelBatch(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrBatchNotFound):
		writeError(w, http.StatusNotFound, "batch not found")
		return
	case errors.Is(err, store.ErrBatchNotActive):
		writeError(w, http.StatusConflict, "the bulk email is not sending (already finished, cancelled, or never sent)")
		return
	case err != nil:
		slog.Error("cancel batch failed", "batch_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to cancel")
		return
	}
	slog.Info("batch cancelled", "batch_id", id, "cancelled", n)
	writeJSON(w, http.StatusOK, cancelResponse{Cancelled: n})
}

type retryResponse struct {
	Retried int `json:"retried"`
}

// RetryBatch handles POST /api/v1/batches/{id}/retry: every failed email of
// the batch goes back in the queue with a fresh set of attempts.
func (h *Handler) RetryBatch(w http.ResponseWriter, r *http.Request) { h.retry(w, r, false) }

// RetryJob handles POST /api/v1/jobs/{id}/retry: one failed email.
func (h *Handler) RetryJob(w http.ResponseWriter, r *http.Request) { h.retry(w, r, true) }

func (h *Handler) retry(w http.ResponseWriter, r *http.Request, singleJob bool) {
	if h.store == nil {
		writeError(w, http.StatusInternalServerError, "database store is not configured")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var n int
	if singleJob {
		n, err = h.store.RetryFailedJob(r.Context(), id)
	} else {
		n, err = h.store.RetryFailedJobs(r.Context(), id, nil)
	}
	switch {
	case errors.Is(err, store.ErrBatchNotFound):
		writeError(w, http.StatusNotFound, "batch not found")
		return
	case errors.Is(err, store.ErrJobNotRetryable):
		writeError(w, http.StatusNotFound, "no failed email with that id")
		return
	case errors.Is(err, store.ErrBatchNotActive):
		writeError(w, http.StatusConflict, "the bulk email was cancelled; its emails cannot be retried")
		return
	case err != nil:
		slog.Error("retry failed", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to retry")
		return
	}
	if n > 0 {
		wake(h.sender)
	}
	writeJSON(w, http.StatusOK, retryResponse{Retried: n})
}

// ---- query parameter helpers ---------------------------------------------

// intParam parses an optional integer query parameter that must be >= minVal.
func intParam(w http.ResponseWriter, v string, def, minVal int, name string) (int, bool) {
	if v == "" {
		return def, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < minVal {
		writeError(w, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return n, true
}

// cursorParam reads ?after= (an ID from next_after; 0 or empty = first page).
func cursorParam(w http.ResponseWriter, q url.Values) (int64, bool) {
	v := q.Get("after")
	if v == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		writeError(w, http.StatusBadRequest, "invalid after")
		return 0, false
	}
	return n, true
}

// oneOf reads an optional parameter that must be one of allowed ("" = any).
func oneOf(w http.ResponseWriter, q url.Values, name string, allowed []string) (string, bool) {
	v := strings.TrimSpace(q.Get(name))
	if v != "" && !slices.Contains(allowed, v) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid %s %q (allowed: %s)", name, v, strings.Join(allowed, ", ")))
		return "", false
	}
	return v, true
}

func searchParam(w http.ResponseWriter, q url.Values) (string, bool) {
	s := strings.TrimSpace(q.Get("q"))
	if len(s) > maxSearchLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("search is too long (max %d characters)", maxSearchLen))
		return "", false
	}
	return s, true
}

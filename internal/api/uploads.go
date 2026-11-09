package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"sendmail/internal/domaincheck"
	"sendmail/internal/recipients"
	"sendmail/internal/store"
)

// UploadStore keeps checked spreadsheets. *store.Store satisfies it; tests
// pass an in-memory fake so the HTTP behaviour can be tested without Postgres.
type UploadStore interface {
	ImportUpload(ctx context.Context, meta store.UploadMeta, fill func(add func(store.UploadRow) error) error) (*store.Upload, error)
	GetUpload(ctx context.Context, id int64) (*store.Upload, error)
	ListUploadRows(ctx context.Context, uploadID int64, f store.UploadRowFilter) ([]store.UploadRow, error)
}

const (
	defaultProblemPage = 100
	maxProblemPage     = 500
	defaultSampleSize  = 5
	maxSampleSize      = 20
	maxSearchLen       = 200
)

// skippedKinds and problemKinds as plain strings, for the store and validation.
var (
	skippedKinds = kindStrings(recipients.SkippedKinds)
	// problemKinds are the kinds the problems endpoint can list: every skipped
	// kind plus "unverified" (sent, but shown so staff know).
	problemKinds = append(kindStrings(recipients.SkippedKinds), string(recipients.KindUnverified))
	sendable     = []string{string(recipients.KindOK), string(recipients.KindUnverified)}
)

func kindStrings(kinds []recipients.Kind) []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

// WithUploadStore sets where checked spreadsheets are kept (tests use a fake)
// and returns h for chaining. NewHandler already uses the database store.
func (h *Handler) WithUploadStore(s UploadStore) *Handler {
	h.uploads = s
	return h
}

// CreateUpload handles POST /api/v1/uploads: check a spreadsheet and keep the
// result. The file is read row by row; every row is saved with its kind
// (ok, duplicate, invalid_email, ...) and the response carries only the
// totals and an upload_id. Problems and sample rows are fetched separately,
// a page at a time, so no response grows with the file.
func (h *Handler) CreateUpload(w http.ResponseWriter, r *http.Request) {
	if h.uploads == nil {
		writeError(w, http.StatusInternalServerError, "database store is not configured")
		return
	}

	start := time.Now()
	file, header, ok := h.readUpload(w, r)
	if !ok {
		return
	}
	defer file.Close()

	// Fingerprint the file (to recognise the same file later), then rewind.
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		writeError(w, http.StatusBadRequest, "could not read the uploaded file")
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, "could not read the uploaded file")
		return
	}

	// The domain check setting is read once, so a change mid-upload cannot
	// treat half the file one way and half the other.
	checkDomains := h.domains != nil && h.domainsOn.Load()
	domainResults := make(map[string]domaincheck.Result) // each domain looked up once per file
	var domainTime time.Duration

	// Tell a bad file (400) apart from a database failure (500).
	var storeErr, parseErr error
	fill := func(add func(store.UploadRow) error) error {
		err := recipients.Scan(file, func(row recipients.Row) error {
			if checkDomains && row.Kind == recipients.KindOK {
				domain := row.Email[strings.LastIndex(row.Email, "@")+1:]
				res, seen := domainResults[domain]
				if !seen {
					t := time.Now()
					res = h.domains.Check(r.Context(), domain)
					domainTime += time.Since(t)
					domainResults[domain] = res
				}
				switch res.Status {
				case domaincheck.Invalid:
					row.Kind, row.Reason = recipients.KindBadDomain, res.Reason
				case domaincheck.Unknown:
					row.Kind, row.Reason = recipients.KindUnverified, res.Reason
				}
			}
			if err := add(store.UploadRow{Row: row.Num, Name: row.Name, Email: row.Email, Kind: string(row.Kind), Reason: row.Reason}); err != nil {
				storeErr = err
				return err
			}
			return nil
		})
		if err != nil && storeErr == nil {
			parseErr = err
		}
		return err
	}

	up, err := h.uploads.ImportUpload(r.Context(), store.UploadMeta{
		Filename:     header.Filename,
		SHA256:       hex.EncodeToString(hash.Sum(nil)),
		SkippedKinds: skippedKinds,
	}, fill)
	switch {
	case err == nil:
	case parseErr == nil:
		slog.Error("save upload failed", "filename", header.Filename, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save the checked file")
		return
	case errors.Is(err, recipients.ErrMissingColumns):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse excel file: %v", err))
		return
	}

	if up.UnverifiedCount > 0 {
		slog.Warn("domain check could not verify some recipients; they are kept", "upload_id", up.ID, "count", up.UnverifiedCount)
	}
	slog.Info("upload checked", "upload_id", up.ID, "filename", up.Filename, "rows", up.TotalRows,
		"valid", up.ValidCount, "problems", up.ProblemCount, "unverified", up.UnverifiedCount,
		"domain_check", ms(domainTime), "total", ms(time.Since(start)))

	writeJSON(w, http.StatusCreated, up)
}

// GetUpload handles GET /api/v1/uploads/{id}: the totals of a checked file.
func (h *Handler) GetUpload(w http.ResponseWriter, r *http.Request) {
	up, ok := h.loadUpload(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, up)
}

// problemPage is one page of problem rows. NextAfter is the value to send as
// ?after= for the next page, or null when this was the last page.
type problemPage struct {
	Items     []store.UploadRow `json:"items"`
	NextAfter *int              `json:"next_after"`
}

// ListUploadProblems handles GET /api/v1/uploads/{id}/problems:
//
//	?kind=duplicate          one or more kinds, comma separated (default: every skipped kind)
//	?q=aisha                 case-insensitive search in name and email
//	?after=52310&limit=100   the page: rows after spreadsheet row 52310
func (h *Handler) ListUploadProblems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	kinds := skippedKinds
	if v := strings.TrimSpace(q.Get("kind")); v != "" {
		kinds = nil
		for _, k := range strings.Split(v, ",") {
			k = strings.TrimSpace(k)
			if !slices.Contains(problemKinds, k) {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid kind %q (allowed: %s)", k, strings.Join(problemKinds, ", ")))
				return
			}
			kinds = append(kinds, k)
		}
	}
	after, ok := intParam(w, q.Get("after"), 0, 0, "after")
	if !ok {
		return
	}
	limit, ok := intParam(w, q.Get("limit"), defaultProblemPage, 1, "limit")
	if !ok {
		return
	}
	limit = min(limit, maxProblemPage)
	search := strings.TrimSpace(q.Get("q"))
	if len(search) > maxSearchLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("search is too long (max %d characters)", maxSearchLen))
		return
	}

	up, ok := h.loadUpload(w, r)
	if !ok {
		return
	}
	// Ask for one extra row: if it comes back, there is a next page.
	rows, err := h.uploads.ListUploadRows(r.Context(), up.ID, store.UploadRowFilter{
		Kinds: kinds, Search: search, After: after, Limit: limit + 1,
	})
	if err != nil {
		slog.Error("list upload problems failed", "upload_id", up.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch problems")
		return
	}
	page := problemPage{Items: rows}
	if len(rows) > limit {
		page.Items = rows[:limit]
		next := page.Items[limit-1].Row
		page.NextAfter = &next
	}
	writeJSON(w, http.StatusOK, page)
}

// sampleResponse holds a few rows that will be sent, for the email preview.
type sampleResponse struct {
	Items []store.UploadRow `json:"items"`
}

// GetUploadSample handles GET /api/v1/uploads/{id}/sample?limit=5: the first
// few rows that will be sent, so the New Bulk Email preview can show real names.
func (h *Handler) GetUploadSample(w http.ResponseWriter, r *http.Request) {
	limit, ok := intParam(w, r.URL.Query().Get("limit"), defaultSampleSize, 1, "limit")
	if !ok {
		return
	}
	limit = min(limit, maxSampleSize)

	up, ok := h.loadUpload(w, r)
	if !ok {
		return
	}
	rows, err := h.uploads.ListUploadRows(r.Context(), up.ID, store.UploadRowFilter{Kinds: sendable, Limit: limit})
	if err != nil {
		slog.Error("upload sample failed", "upload_id", up.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch sample rows")
		return
	}
	writeJSON(w, http.StatusOK, sampleResponse{Items: rows})
}

// loadUpload reads {id} from the path and fetches the upload, writing the
// error response itself when that fails.
func (h *Handler) loadUpload(w http.ResponseWriter, r *http.Request) (*store.Upload, bool) {
	if h.uploads == nil {
		writeError(w, http.StatusInternalServerError, "database store is not configured")
		return nil, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid upload id")
		return nil, false
	}
	up, err := h.uploads.GetUpload(r.Context(), id)
	if errors.Is(err, store.ErrUploadNotFound) {
		writeError(w, http.StatusNotFound, "upload not found (checked files are kept for 24 hours)")
		return nil, false
	}
	if err != nil {
		slog.Error("get upload failed", "upload_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch upload")
		return nil, false
	}
	return up, true
}

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

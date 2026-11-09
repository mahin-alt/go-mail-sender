package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"sendmail/internal/domaincheck"
	"sendmail/internal/logging"
	"sendmail/internal/store"
)

// fakeUploads is an in-memory UploadStore, so the HTTP behaviour can be tested
// without Postgres (the SQL itself is tested in internal/store).
type fakeUploads struct {
	mu      sync.Mutex
	next    int64
	uploads map[int64]*store.Upload
	rows    map[int64][]store.UploadRow
	failAdd error // when set, every add returns it (a failing database)
}

func newFakeUploads() *fakeUploads {
	return &fakeUploads{uploads: map[int64]*store.Upload{}, rows: map[int64][]store.UploadRow{}}
}

func (f *fakeUploads) ImportUpload(_ context.Context, meta store.UploadMeta, fill func(add func(store.UploadRow) error) error) (*store.Upload, error) {
	var rows []store.UploadRow
	if err := fill(func(r store.UploadRow) error {
		if f.failAdd != nil {
			return f.failAdd
		}
		rows = append(rows, r)
		return nil
	}); err != nil {
		return nil, err
	}

	up := &store.Upload{Filename: meta.Filename, Status: "ready", ProblemCounts: map[string]int{},
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(24 * time.Hour)}
	for _, k := range meta.SkippedKinds {
		up.ProblemCounts[k] = 0
	}
	for _, r := range rows {
		up.TotalRows++
		if slices.Contains(meta.SkippedKinds, r.Kind) {
			up.ProblemCount++
			up.ProblemCounts[r.Kind]++
		} else {
			up.ValidCount++
			if r.Kind == "unverified" {
				up.UnverifiedCount++
			}
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	up.ID = f.next
	f.uploads[up.ID] = up
	f.rows[up.ID] = rows
	return up, nil
}

func (f *fakeUploads) GetUpload(_ context.Context, id int64) (*store.Upload, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	up, ok := f.uploads[id]
	if !ok {
		return nil, store.ErrUploadNotFound
	}
	return up, nil
}

func (f *fakeUploads) ListUploadRows(_ context.Context, id int64, flt store.UploadRowFilter) ([]store.UploadRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []store.UploadRow{}
	q := strings.ToLower(flt.Search)
	for _, r := range f.rows[id] {
		if !slices.Contains(flt.Kinds, r.Kind) || r.Row <= flt.After {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(r.Name), q) && !strings.Contains(strings.ToLower(r.Email), q) {
			continue
		}
		out = append(out, r)
		if len(out) == flt.Limit {
			break
		}
	}
	return out, nil
}

func newUploadHandler() *Handler {
	return NewHandler(&fakeMailer{}, nil, nil).WithUploadStore(newFakeUploads())
}

func sample(name string) string { return filepath.Join("..", "..", "samples", name) }

// upload posts a sample file to POST /api/v1/uploads and decodes the answer.
func upload(t *testing.T, h *Handler, file string) store.Upload {
	t.Helper()
	rec := postFileTo(h, "/api/v1/uploads", sample(file), file, "file")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body)
	}
	var up store.Upload
	if err := json.NewDecoder(rec.Body).Decode(&up); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return up
}

func get(h *Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	NewRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestCreateUpload(t *testing.T) {
	t.Run("returns totals only, never the rows", func(t *testing.T) {
		h := newUploadHandler()
		rec := postFileTo(h, "/api/v1/uploads", sample("9_mixed_everything.xlsx"), "9_mixed_everything.xlsx", "file")
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body)
		}
		body := rec.Body.String()
		for _, leak := range []string{"aisha@example.com", `"valid":`, `"problems":`} {
			if strings.Contains(body, leak) {
				t.Errorf("response must not contain %q: %s", leak, body)
			}
		}

		var up store.Upload
		if err := json.Unmarshal([]byte(body), &up); err != nil {
			t.Fatal(err)
		}
		if up.ID == 0 || up.Filename != "9_mixed_everything.xlsx" || up.Status != "ready" {
			t.Errorf("upload = %+v", up)
		}
		if up.TotalRows != 8 || up.ValidCount != 3 || up.ProblemCount != 5 || up.UnverifiedCount != 0 {
			t.Errorf("totals = %d/%d/%d/%d, want 8 rows, 3 valid, 5 problems, 0 unverified",
				up.TotalRows, up.ValidCount, up.ProblemCount, up.UnverifiedCount)
		}
		want := map[string]int{"empty_email": 1, "empty_name": 1, "invalid_email": 2, "duplicate": 1, "bad_domain": 0}
		for k, n := range want {
			if up.ProblemCounts[k] != n {
				t.Errorf("problem_counts[%s] = %d, want %d (all: %v)", k, up.ProblemCounts[k], n, up.ProblemCounts)
			}
		}
	})

	t.Run("missing form field", func(t *testing.T) {
		if rec := postFileTo(newUploadHandler(), "/api/v1/uploads", "", "", ""); rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("unsupported file extension", func(t *testing.T) {
		if rec := postFileTo(newUploadHandler(), "/api/v1/uploads", "", "test.csv", "file"); rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("wrong headers", func(t *testing.T) {
		rec := postFileTo(newUploadHandler(), "/api/v1/uploads", sample("7_wrong_headers.xlsx"), "7_wrong_headers.xlsx", "file")
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "name") {
			t.Errorf("status = %d body = %s, want 400 naming the columns", rec.Code, rec.Body)
		}
	})

	t.Run("a database failure is a 500, not a bad file", func(t *testing.T) {
		fake := newFakeUploads()
		fake.failAdd = errors.New("connection lost")
		h := NewHandler(&fakeMailer{}, nil, nil).WithUploadStore(fake)
		rec := postFileTo(h, "/api/v1/uploads", sample("1_valid.xlsx"), "1_valid.xlsx", "file")
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "connection lost") {
			t.Errorf("status = %d body = %s, want 500 without internal details", rec.Code, rec.Body)
		}
	})

	t.Run("no store configured", func(t *testing.T) {
		rec := postFileTo(NewHandler(&fakeMailer{}, nil, nil), "/api/v1/uploads", sample("1_valid.xlsx"), "1_valid.xlsx", "file")
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rec.Code)
		}
	})
}

func TestCreateUpload_DomainCheck(t *testing.T) {
	checker := fakeDomains{
		"nonexistent.invalid": {Status: domaincheck.Invalid, Reason: "domain nonexistent.invalid does not exist or has no mail server"},
		"example.com":         {Status: domaincheck.Invalid, Reason: "domain example.com does not accept mail"},
		"yahoo.com":           {Status: domaincheck.Unknown, Reason: "could not verify domain yahoo.com: timeout"},
	}

	t.Run("dead domains skipped, unknown kept and counted", func(t *testing.T) {
		h := newUploadHandler().WithDomainChecker(checker)
		up := upload(t, h, "11_domain_check.xlsx")
		if up.ProblemCounts["bad_domain"] != 2 || up.ProblemCount != 2 {
			t.Errorf("problems = %d (%v), want 2 bad_domain", up.ProblemCount, up.ProblemCounts)
		}
		if up.ValidCount != 6 || up.UnverifiedCount != 1 {
			t.Errorf("valid = %d unverified = %d, want 6 and 1", up.ValidCount, up.UnverifiedCount)
		}

		rec := get(h, fmt.Sprintf("/api/v1/uploads/%d/problems?kind=unverified", up.ID))
		var page problemPage
		json.NewDecoder(rec.Body).Decode(&page)
		if len(page.Items) != 1 || page.Items[0].Email != "carla@yahoo.com" || page.Items[0].Reason == "" {
			t.Errorf("unverified rows = %+v, want just carla@yahoo.com with a reason", page.Items)
		}
	})

	t.Run("switched off at runtime leaves everything valid", func(t *testing.T) {
		h := newUploadHandler().WithDomainChecker(checker)
		h.SetDomainCheckEnabled(false)
		up := upload(t, h, "11_domain_check.xlsx")
		if up.ValidCount != 8 || up.ProblemCount != 0 || up.UnverifiedCount != 0 {
			t.Errorf("valid=%d problems=%d unverified=%d, want 8/0/0", up.ValidCount, up.ProblemCount, up.UnverifiedCount)
		}
	})
}

func TestCreateUpload_LogsSummary(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(logging.New(&buf, slog.LevelInfo, "text"))
	t.Cleanup(func() { slog.SetDefault(prev) })

	upload(t, newUploadHandler(), "9_mixed_everything.xlsx")
	line := buf.String()
	for _, part := range []string{`msg="upload checked"`, "upload_id=", "valid=3", "problems=5", "domain_check=", "total="} {
		if !strings.Contains(line, part) {
			t.Errorf("log line missing %q:\n%s", part, line)
		}
	}
}

func TestGetUpload(t *testing.T) {
	h := newUploadHandler()
	up := upload(t, h, "9_mixed_everything.xlsx")

	rec := get(h, fmt.Sprintf("/api/v1/uploads/%d", up.ID))
	var got store.Upload
	if rec.Code != http.StatusOK || json.NewDecoder(rec.Body).Decode(&got) != nil || got.ProblemCount != 5 {
		t.Errorf("status = %d upload = %+v", rec.Code, got)
	}
	if rec := get(h, "/api/v1/uploads/999"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown id: status = %d, want 404", rec.Code)
	}
	if rec := get(h, "/api/v1/uploads/abc"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: status = %d, want 400", rec.Code)
	}
}

func TestListUploadProblems(t *testing.T) {
	h := newUploadHandler()
	up := upload(t, h, "9_mixed_everything.xlsx") // problems on rows 3, 4, 5, 8, 9
	base := fmt.Sprintf("/api/v1/uploads/%d/problems", up.ID)

	page := func(t *testing.T, query string) problemPage {
		t.Helper()
		rec := get(h, base+query)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d (body: %s)", query, rec.Code, rec.Body)
		}
		var p problemPage
		if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	rowsOf := func(p problemPage) []int {
		var out []int
		for _, r := range p.Items {
			out = append(out, r.Row)
		}
		return out
	}

	t.Run("default lists every skipped row in row order", func(t *testing.T) {
		p := page(t, "")
		if got := rowsOf(p); !slices.Equal(got, []int{3, 4, 5, 8, 9}) || p.NextAfter != nil {
			t.Errorf("rows = %v next = %v, want [3 4 5 8 9] and no next page", got, p.NextAfter)
		}
		if p.Items[0].Kind != "empty_email" || p.Items[0].Reason == "" {
			t.Errorf("first row = %+v, want empty_email with a reason", p.Items[0])
		}
	})

	t.Run("pages follow next_after", func(t *testing.T) {
		p1 := page(t, "?limit=2")
		if got := rowsOf(p1); !slices.Equal(got, []int{3, 4}) || p1.NextAfter == nil || *p1.NextAfter != 4 {
			t.Fatalf("page 1 = %v next = %v, want [3 4] next 4", got, p1.NextAfter)
		}
		p2 := page(t, "?limit=2&after=4")
		p3 := page(t, fmt.Sprintf("?limit=2&after=%d", *p2.NextAfter))
		if got := append(rowsOf(p2), rowsOf(p3)...); !slices.Equal(got, []int{5, 8, 9}) || p3.NextAfter != nil {
			t.Errorf("pages 2+3 = %v, last next = %v, want [5 8 9] and no next page", got, p3.NextAfter)
		}
	})

	t.Run("filter by kinds", func(t *testing.T) {
		if got := rowsOf(page(t, "?kind=invalid_email")); !slices.Equal(got, []int{4, 9}) {
			t.Errorf("invalid_email rows = %v, want [4 9]", got)
		}
		if got := rowsOf(page(t, "?kind=empty_email,empty_name,invalid_email")); !slices.Equal(got, []int{3, 4, 5, 9}) {
			t.Errorf("Invalid tab rows = %v, want [3 4 5 9]", got)
		}
	})

	t.Run("search name or email", func(t *testing.T) {
		if got := rowsOf(page(t, "?q=AISHA")); !slices.Equal(got, []int{8}) {
			t.Errorf("search rows = %v, want the duplicate on row 8", got)
		}
	})

	t.Run("bad parameters", func(t *testing.T) {
		for _, q := range []string{"?kind=ok", "?kind=nope", "?after=-1", "?limit=0", "?limit=x", "?q=" + strings.Repeat("a", 201)} {
			if rec := get(h, base+q); rec.Code != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400", q, rec.Code)
			}
		}
		if rec := get(h, "/api/v1/uploads/999/problems"); rec.Code != http.StatusNotFound {
			t.Errorf("unknown upload: status = %d, want 404", rec.Code)
		}
	})
}

func TestGetUploadSample(t *testing.T) {
	h := newUploadHandler()
	up := upload(t, h, "9_mixed_everything.xlsx") // valid rows 2, 6, 10

	rec := get(h, fmt.Sprintf("/api/v1/uploads/%d/sample?limit=2", up.ID))
	var got sampleResponse
	if rec.Code != http.StatusOK || json.NewDecoder(rec.Body).Decode(&got) != nil {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body)
	}
	if len(got.Items) != 2 || got.Items[0].Row != 2 || got.Items[1].Row != 6 || got.Items[0].Name != "Aisha Rahman" {
		t.Errorf("sample = %+v, want rows 2 and 6", got.Items)
	}
	if rec := get(h, fmt.Sprintf("/api/v1/uploads/%d/sample?limit=0", up.ID)); rec.Code != http.StatusBadRequest {
		t.Errorf("limit=0: status = %d, want 400", rec.Code)
	}
}

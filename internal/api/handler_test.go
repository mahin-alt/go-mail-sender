package api

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sendmail/internal/importer"
)

// fakeMailer satisfies mailer.Mailer without touching the network.
// This is the payoff of the interface: handlers are testable without SMTP.
type fakeMailer struct {
	err   error
	calls int
}

func (f *fakeMailer) Send(to, subject, body string) error {
	f.calls++
	return f.err
}

// post sends a request through the real router using an in-memory recorder
// (httptest) - no actual port is opened.
func post(m *fakeMailer, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/emails", strings.NewReader(body))
	rec := httptest.NewRecorder()
	NewRouter(NewHandler(m, nil, nil)).ServeHTTP(rec, req)
	return rec
}

func TestSendEmail(t *testing.T) {
	good := `{"to":"a@example.com","subject":"hi","body":"hello"}`

	tests := []struct {
		name      string
		body      string
		mailerErr error
		wantCode  int
		wantSent  int
	}{
		{"valid", good, nil, http.StatusOK, 1},
		{"malformed json", `{`, nil, http.StatusBadRequest, 0},
		{"unknown field", `{"to":"a@example.com","subject":"s","body":"b","x":1}`, nil, http.StatusBadRequest, 0},
		{"bad address", `{"to":"nope","subject":"s","body":"b"}`, nil, http.StatusBadRequest, 0},
		{"empty subject", `{"to":"a@example.com","subject":"","body":"b"}`, nil, http.StatusBadRequest, 0},
		{"header injection", `{"to":"a@example.com","subject":"s\r\nBcc: x@y.com","body":"b"}`, nil, http.StatusBadRequest, 0},
		{"smtp failure", good, errors.New("boom"), http.StatusBadGateway, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &fakeMailer{err: tc.mailerErr}
			rec := post(m, tc.body)
			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tc.wantCode, rec.Body)
			}
			if m.calls != tc.wantSent {
				t.Errorf("mailer calls = %d, want %d", m.calls, tc.wantSent)
			}
		})
	}
}

func TestWrongMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/emails", nil)
	rec := httptest.NewRecorder()
	NewRouter(NewHandler(&fakeMailer{}, nil, nil)).ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

// uploadHandler is a handler without a database whose uploads go to a
// temporary folder: enough for every check that happens before the batch is
// created.
func uploadHandler(t *testing.T) (*Handler, importer.Files) {
	files := importer.Files{Dir: t.TempDir()}
	return NewHandler(&fakeMailer{}, nil, nil).WithImports(files, nil, nil), files
}

func postFileTo(h *Handler, route, path, filename, fieldName string) *httptest.ResponseRecorder {
	var data []byte
	if path != "" {
		data, _ = os.ReadFile(path)
	}
	return postForm(h, route, fieldName, filename, data)
}

// postBytes uploads data as the "file" field of a multipart form.
func postBytes(h *Handler, route, filename string, data []byte) *httptest.ResponseRecorder {
	return postForm(h, route, "file", filename, data)
}

func postForm(h *Handler, route, field, filename string, data []byte) *httptest.ResponseRecorder {
	return postFormTo(NewRouter(h), route, field, filename, data)
}

func postFormTo(router http.Handler, route, field, filename string, data []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("note", "fields before the file are skipped")
	if field != "" {
		part, _ := writer.CreateFormFile(field, filename)
		part.Write(data)
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, route, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// zipLike is n bytes that start like a ZIP (.xlsx) file, so they pass the
// format check and reach the size and database steps.
func zipLike(n int) []byte {
	b := make([]byte, n)
	copy(b, "PK\x03\x04")
	return b
}

func TestUpload_Validation(t *testing.T) {
	sample := filepath.Join("..", "..", "samples", "1_valid.xlsx")
	for _, route := range []string{"/api/v1/verify", "/api/v1/batches"} {
		t.Run(route, func(t *testing.T) {
			h, files := uploadHandler(t)
			cases := []struct {
				name     string
				rec      *httptest.ResponseRecorder
				wantCode int
				wantText string
			}{
				{"no file field", postForm(h, route, "", "", nil), 400, `file\" is required`},
				{"wrong field name", postFileTo(h, route, sample, "1_valid.xlsx", "upload"), 400, `file\" is required`},
				{"not .xlsx by name", postFileTo(h, route, sample, "people.csv", "file"), 400, "only .xlsx"},
				{"not .xlsx inside", postBytes(h, route, "fake.xlsx", []byte("name,email\n")), 400, "not a valid .xlsx"},
				// A real spreadsheet passes every check; without a database it
				// stops at the batch step.
				{"valid file, no database", postFileTo(h, route, sample, "1_valid.xlsx", "file"), 500, "store is not configured"},
			}
			for _, c := range cases {
				if c.rec.Code != c.wantCode || !strings.Contains(c.rec.Body.String(), c.wantText) {
					t.Errorf("%s: status = %d body = %s, want %d containing %q", c.name, c.rec.Code, c.rec.Body, c.wantCode, c.wantText)
				}
			}
			// Every rejected upload cleaned up after itself.
			if entries, _ := os.ReadDir(files.Dir); len(entries) != 0 {
				t.Errorf("upload folder keeps %d files", len(entries))
			}
		})
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/verify", strings.NewReader(`{"file":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h, _ := uploadHandler(t)
	NewRouter(h).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("JSON instead of a form: status = %d", rec.Code)
	}
}

func TestUploadSizeLimit(t *testing.T) {
	const mb = 1 << 20
	route := "/api/v1/batches"

	t.Run("default is 10 MB", func(t *testing.T) {
		h, _ := uploadHandler(t)
		rec := postBytes(h, route, "big.xlsx", zipLike(10*mb+1))
		if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "10 MB") {
			t.Fatalf("status = %d body = %s, want 413 naming 10 MB", rec.Code, rec.Body)
		}
	})

	t.Run("a file exactly at the limit is not rejected for size", func(t *testing.T) {
		h, _ := uploadHandler(t)
		h.SetMaxUploadMB(1)
		if rec := postBytes(h, route, "edge.xlsx", zipLike(mb)); rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d body = %s, want 500 from the missing database (size accepted)", rec.Code, rec.Body)
		}
	})

	t.Run("changes apply at runtime without a new handler", func(t *testing.T) {
		h, _ := uploadHandler(t)
		twoMB := zipLike(2 * mb)

		h.SetMaxUploadMB(1)
		if rec := postBytes(h, route, "f.xlsx", twoMB); rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "1 MB") {
			t.Fatalf("limit 1: status = %d body = %s, want 413 naming 1 MB", rec.Code, rec.Body)
		}
		h.SetMaxUploadMB(3)
		if rec := postBytes(h, route, "f.xlsx", twoMB); rec.Code != http.StatusInternalServerError {
			t.Fatalf("limit 3: status = %d body = %s, want the size accepted", rec.Code, rec.Body)
		}
		h.SetMaxUploadMB(0) // ignored
		if rec := postBytes(h, route, "f.xlsx", twoMB); rec.Code != http.StatusInternalServerError {
			t.Fatalf("after ignored 0: status = %d, want the limit to stay 3 MB", rec.Code)
		}
	})
}

func TestCleanFilename(t *testing.T) {
	for in, want := range map[string]string{
		"people.xlsx":                      "people.xlsx",
		`C:\Users\me\people.xlsx`:          "people.xlsx",
		"../../etc/passwd.xlsx":            "passwd.xlsx",
		"":                                 "",
		strings.Repeat("a", 300) + ".xlsx": strings.Repeat("a", 250) + ".xlsx",
	} {
		if got := cleanFilename(in); got != want {
			t.Errorf("cleanFilename(%.40q) = %.40q, want %.40q", in, got, want)
		}
	}
}

func TestValidateTemplates(t *testing.T) {
	for _, c := range []struct{ subject, body, want string }{
		{"Hi {{.Name}}", "Hello {{.Email}}", ""},
		{"", "b", `"subject" is required`},
		{"s", "", `"body" is required`},
		{"Hi {{.Phone}}", "b", "invalid subject template"},
		{"s", "Dear {{.Name", "invalid body template"},
		{"line one\nline two", "b", "one line"},
		{strings.Repeat("s", 999), "b", "too long"},
	} {
		got := validateTemplates(c.subject, c.body)
		if (c.want == "" && got != "") || (c.want != "" && !strings.Contains(got, c.want)) {
			t.Errorf("validateTemplates(%.20q, %.20q) = %q, want %q", c.subject, c.body, got, c.want)
		}
	}
}

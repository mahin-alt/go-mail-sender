package api

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sendmail/internal/domaincheck"
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

func postFile(route, path, filename, fieldName string) *httptest.ResponseRecorder {
	return postFileTo(NewHandler(&fakeMailer{}, nil, nil), route, path, filename, fieldName)
}

func postFileTo(h *Handler, route, path, filename, fieldName string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if fieldName != "" {
		part, _ := writer.CreateFormFile(fieldName, filename)
		if path != "" {
			data, _ := os.ReadFile(path)
			part.Write(data)
		}
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, route, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	NewRouter(h).ServeHTTP(rec, req)
	return rec
}

// fakeDomains satisfies DomainChecker with canned answers per domain
// (anything not listed is Valid), so no DNS is needed.
type fakeDomains map[string]domaincheck.Result

func (f fakeDomains) Check(_ context.Context, domain string) domaincheck.Result {
	if r, ok := f[domain]; ok {
		return r
	}
	return domaincheck.Result{Status: domaincheck.Valid}
}

// postBytes uploads data as the "file" field of a multipart form.
func postBytes(h *Handler, route, filename string, data []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", filename)
	part.Write(data)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, route, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	NewRouter(h).ServeHTTP(rec, req)
	return rec
}

func TestUploadSizeLimit(t *testing.T) {
	const mb = 1 << 20
	route := "/api/v1/uploads"

	t.Run("default is 10 MB", func(t *testing.T) {
		rec := postBytes(newUploadHandler(), route, "big.xlsx", make([]byte, 10*mb+1))
		if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "10 MB") {
			t.Fatalf("status = %d body = %s, want 413 naming 10 MB", rec.Code, rec.Body)
		}
	})

	t.Run("a file exactly at the limit is not rejected for size", func(t *testing.T) {
		h := newUploadHandler()
		h.SetMaxUploadMB(1)
		// Not a real spreadsheet, so it gets past the size check and fails parsing.
		if rec := postBytes(h, route, "edge.xlsx", make([]byte, mb)); rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d body = %s, want 400 from parsing", rec.Code, rec.Body)
		}
	})

	t.Run("changes apply at runtime without a new handler", func(t *testing.T) {
		h := newUploadHandler()
		twoMB := make([]byte, 2*mb)

		h.SetMaxUploadMB(1)
		if rec := postBytes(h, route, "f.xlsx", twoMB); rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "1 MB") {
			t.Fatalf("limit 1: status = %d body = %s, want 413 naming 1 MB", rec.Code, rec.Body)
		}

		h.SetMaxUploadMB(3)
		if rec := postBytes(h, route, "f.xlsx", twoMB); rec.Code != http.StatusBadRequest {
			t.Fatalf("limit 3: status = %d body = %s, want 400 from parsing (size accepted)", rec.Code, rec.Body)
		}

		h.SetMaxUploadMB(0) // ignored
		if rec := postBytes(h, route, "f.xlsx", twoMB); rec.Code != http.StatusBadRequest {
			t.Fatalf("after ignored 0: status = %d, want the limit to stay 3 MB", rec.Code)
		}
	})

	t.Run("real spreadsheet under a raised limit parses", func(t *testing.T) {
		h := newUploadHandler()
		h.SetMaxUploadMB(40)
		rec := postFileTo(h, route, filepath.Join("..", "..", "samples", "1_valid.xlsx"), "1_valid.xlsx", "file")
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
		}
	})
}

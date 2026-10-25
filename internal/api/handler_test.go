package api

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestParseRecipients(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		rec := postFile("/api/v1/recipients/parse", filepath.Join("..", "..", "samples", "1_valid.xlsx"), "1_valid.xlsx", "file")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
		}

		var resp parseRecipientsResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(resp.Valid) != 5 {
			t.Errorf("valid = %d, want 5", len(resp.Valid))
		}
		if len(resp.Problems) != 0 {
			t.Errorf("problems = %d, want 0", len(resp.Problems))
		}
	})

	t.Run("missing form field", func(t *testing.T) {
		rec := postFile("/api/v1/recipients/parse", "", "", "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("unsupported file extension", func(t *testing.T) {
		rec := postFile("/api/v1/recipients/parse", "", "test.csv", "file")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("wrong headers", func(t *testing.T) {
		rec := postFile("/api/v1/recipients/parse", filepath.Join("..", "..", "samples", "7_wrong_headers.xlsx"), "7_wrong_headers.xlsx", "file")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 (body: %s)", rec.Code, rec.Body)
		}
	})
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

func TestParseRecipientsDomainCheck(t *testing.T) {
	sample := filepath.Join("..", "..", "samples", "11_domain_check.xlsx")
	checker := fakeDomains{
		"nonexistent.invalid": {Status: domaincheck.Invalid, Reason: "domain nonexistent.invalid does not exist or has no mail server"},
		"example.com":         {Status: domaincheck.Invalid, Reason: "domain example.com does not accept mail"},
		"yahoo.com":           {Status: domaincheck.Unknown, Reason: "could not verify domain yahoo.com: timeout"},
	}

	parse := func(t *testing.T, h *Handler) parseRecipientsResponse {
		t.Helper()
		rec := postFileTo(h, "/api/v1/recipients/parse", sample, "11_domain_check.xlsx", "file")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
		}
		var resp parseRecipientsResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return resp
	}

	t.Run("dead domains rejected, unknown kept and reported", func(t *testing.T) {
		resp := parse(t, NewHandler(&fakeMailer{}, nil, nil).WithDomainChecker(checker))

		if len(resp.Problems) != 2 {
			t.Fatalf("problems = %d, want 2 (%+v)", len(resp.Problems), resp.Problems)
		}
		for _, p := range resp.Problems {
			if p.Row < 2 || p.Reason == "" {
				t.Errorf("problem should carry its row and reason: %+v", p)
			}
		}
		if resp.Problems[0].Row > resp.Problems[1].Row {
			t.Errorf("problems should be in row order: %+v", resp.Problems)
		}
		if len(resp.Valid) != 6 {
			t.Errorf("valid = %d, want 6", len(resp.Valid))
		}
		if resp.UnverifiedCount != 1 || len(resp.Unverified) != 1 || resp.Unverified[0].Email != "carla@yahoo.com" {
			t.Errorf("unverified = %d %+v, want just carla@yahoo.com", resp.UnverifiedCount, resp.Unverified)
		}
	})

	t.Run("no checker configured leaves everything valid", func(t *testing.T) {
		resp := parse(t, NewHandler(&fakeMailer{}, nil, nil))
		if len(resp.Valid) != 8 || len(resp.Problems) != 0 || resp.UnverifiedCount != 0 {
			t.Errorf("valid=%d problems=%d unverified=%d, want 8/0/0", len(resp.Valid), len(resp.Problems), resp.UnverifiedCount)
		}
	})
}

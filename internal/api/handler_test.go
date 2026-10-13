package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	NewRouter(NewHandler(m)).ServeHTTP(rec, req)
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
	NewRouter(NewHandler(&fakeMailer{})).ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

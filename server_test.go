package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoadMailtrapConfig(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		fromEmail string
		wantError bool
	}{
		{name: "missing token", fromEmail: "hello@demomailtrap.co", wantError: true},
		{name: "placeholder token", token: "your_mailtrap_api_token", fromEmail: "hello@demomailtrap.co", wantError: true},
		{name: "missing sender", token: "test-token", wantError: true},
		{name: "invalid sender", token: "test-token", fromEmail: "not-an-email", wantError: true},
		{name: "valid config", token: "test-token", fromEmail: "hello@demomailtrap.co"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("MAILTRAP_API_TOKEN", test.token)
			t.Setenv("FROM_EMAIL", test.fromEmail)

			_, err := loadMailtrapConfig()
			if (err != nil) != test.wantError {
				t.Fatalf("loadMailtrapConfig() error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func TestSendMailRequiresMailtrapConfig(t *testing.T) {
	t.Setenv("MAILTRAP_API_TOKEN", "")
	t.Setenv("FROM_EMAIL", "hello@demomailtrap.co")

	if err := sendMailSimple(context.Background(), "Test", "Test body", []string{"recipient@example.com"}); err == nil {
		t.Fatal("sendMailSimple() succeeded without an API token")
	}
}

func TestSendMailRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name   string
		method string
		body   string
		want   int
	}{
		{
			name:   "method other than POST",
			method: http.MethodGet,
			want:   http.StatusMethodNotAllowed,
		},
		{
			name:   "malformed JSON",
			method: http.MethodPost,
			body:   "{",
			want:   http.StatusBadRequest,
		},
		{
			name:   "missing required fields",
			method: http.MethodPost,
			body:   `{"subject":"Hi","body":"Hello","to":[]}`,
			want:   http.StatusBadRequest,
		},
		{
			name:   "newline in subject",
			method: http.MethodPost,
			body:   `{"subject":"Hi\r\nBcc: other@example.com","body":"Hello","to":["you@example.com"]}`,
			want:   http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "/send-mail", strings.NewReader(test.body))
			response := httptest.NewRecorder()

			SendMail(response, request)

			if response.Code != test.want {
				t.Errorf("status = %d, want %d", response.Code, test.want)
			}
		})
	}
}

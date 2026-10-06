package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

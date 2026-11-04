package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sendmail/internal/logging"
)

func TestWithRequestLog(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelDebug, "text")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/boom":
			w.WriteHeader(http.StatusInternalServerError)
		case "/bad":
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		default:
			w.Write([]byte("ok")) // no explicit WriteHeader means 200
		}
	})
	h := WithRequestLog(next, logger)

	call := func(method, path string) *httptest.ResponseRecorder {
		buf.Reset()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec
	}

	cases := []struct {
		method, path, want string
	}{
		{http.MethodGet, "/api/v1/batches", "level=DEBUG msg=request"}, // polling reads stay out of info
		{http.MethodPost, "/api/v1/batches", "level=INFO msg=request"},
		{http.MethodPut, "/api/v1/settings", "level=INFO msg=request"},
		{http.MethodPost, "/bad", "level=WARN msg=request"},
		{http.MethodGet, "/bad", "level=WARN msg=request"}, // a failed read is still visible at info
		{http.MethodGet, "/boom", "level=ERROR msg=request"},
		{http.MethodOptions, "/api/v1/batches", "level=DEBUG msg=request"},
	}
	for _, c := range cases {
		rec := call(c.method, c.path)
		line := buf.String()
		id := rec.Header().Get("X-Request-ID")
		if len(id) != 8 {
			t.Errorf("%s %s: X-Request-ID = %q, want 8 hex chars", c.method, c.path, id)
		}
		for _, part := range []string{c.want, "req_id=" + id, "method=" + c.method, "path=" + c.path, "duration="} {
			if !strings.Contains(line, part) {
				t.Errorf("%s %s: log line missing %q:\n%s", c.method, c.path, part, line)
			}
		}
	}

	if !strings.Contains(func() string { call(http.MethodGet, "/boom"); return buf.String() }(), "status=500") {
		t.Error("status should be recorded")
	}

	rec := call(http.MethodGet, "/healthz")
	if buf.Len() != 0 || rec.Header().Get("X-Request-ID") != "" {
		t.Errorf("/healthz should not be logged: %q", buf.String())
	}
}

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func corsServer(origins ...string) http.Handler {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	return WithCORS(next, origins)
}

func TestWithCORS_AllowedOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	corsServer("http://localhost:5173/").ServeHTTP(rec, req) // trailing slash is trimmed

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("Allow-Origin = %q", got)
	}
	if rec.Code != http.StatusTeapot {
		t.Fatalf("request should reach next handler, got %d", rec.Code)
	}
}

func TestWithCORS_DisallowedOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	corsServer("http://localhost:5173").ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Allow-Origin should be empty, got %q", got)
	}
}

func TestWithCORS_Preflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/batches", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	corsServer("*").ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Fatal("preflight missing Allow-Headers")
	}
}

func TestListBatches_NoStore(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/batches", nil)
	rec := httptest.NewRecorder()
	NewRouter(NewHandler(&fakeMailer{}, nil, nil)).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 when store is not configured", rec.Code)
	}
}

func TestRetryEndpoints_NoStore(t *testing.T) {
	for _, path := range []string{"/api/v1/batches/1/retry", "/api/v1/jobs/1/retry"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		NewRouter(NewHandler(&fakeMailer{}, nil, nil)).ServeHTTP(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want 500 when store is not configured", path, rec.Code)
		}
	}
}

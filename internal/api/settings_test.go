package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sendmail/internal/config"
	"sendmail/internal/settings"
)

// settingsRouter builds a router whose settings service has no database behind
// it. That is enough for listing and for every request that is rejected during
// validation, which happens before anything is stored.
func settingsRouter() http.Handler {
	cfg := config.Config{
		SMTPHost: "localhost", SMTPPort: "1025", SMTPTLSMode: "auto", MailFrom: "env@example.com",
		SMTPPassword: "super-secret-pw", MaxJobRetries: 3, WorkerCount: 3, SMTPPoolSize: 3,
		MaxUploadMB:     config.DefaultMaxUploadMB,
		ImportChunkSize: config.DefaultImportChunkSize, JobGenerationChunkSize: config.DefaultJobGenerationChunkSize,
	}
	svc := settings.New(nil, cfg, nil, nil)
	return NewRouter(NewHandler(&fakeMailer{}, nil, nil).WithSettings(svc))
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSettingsEndpoints_NotConfigured(t *testing.T) {
	r := NewRouter(NewHandler(&fakeMailer{}, nil, nil))
	for _, c := range [][2]string{
		{"GET", "/api/v1/settings"},
		{"PUT", "/api/v1/settings"},
		{"POST", "/api/v1/settings/smtp/test"},
	} {
		if rec := do(r, c[0], c[1], `{}`); rec.Code != http.StatusInternalServerError {
			t.Errorf("%s %s: status = %d, want 500", c[0], c[1], rec.Code)
		}
	}
}

func TestGetSettings_ListsItemsAndNeverLeaksSecrets(t *testing.T) {
	rec := do(settingsRouter(), "GET", "/api/v1/settings", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "super-secret-pw") {
		t.Fatal("SMTP password leaked in GET /settings")
	}

	var resp struct {
		Items []struct {
			Key      string `json:"key"`
			Value    string `json:"value"`
			Secret   bool   `json:"secret"`
			HasValue bool   `json:"has_value"`
			Editable bool   `json:"editable"`
			Apply    string `json:"apply"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, it := range resp.Items {
		seen[it.Key] = true
		switch it.Key {
		case "SMTP_PASSWORD":
			if !it.Secret || it.Value != "" || !it.HasValue {
				t.Errorf("password item wrong: %+v", it)
			}
		case "HTTP_ADDR":
			if it.Editable {
				t.Errorf("HTTP_ADDR must be read-only")
			}
		case "MAIL_FROM":
			if it.Value != "env@example.com" || !it.Editable || it.Apply != "instant" {
				t.Errorf("MAIL_FROM wrong: %+v", it)
			}
		}
	}
	if seen["DATABASE_URL"] {
		t.Fatal("DATABASE_URL must not be listed")
	}
	for _, k := range []string{"MAIL_FROM", "SMTP_HOST", "SMTP_PASSWORD", "IMPORT_CHUNK_SIZE", "JOB_GENERATION_CHUNK_SIZE", "WORKER_COUNT", "HTTP_ADDR"} {
		if !seen[k] {
			t.Errorf("missing %s", k)
		}
	}
}

func TestUpdateSettings_RejectsBadRequests(t *testing.T) {
	r := settingsRouter()
	cases := []struct{ name, body string }{
		{"not json", `nope`},
		{"unknown top-level field", `{"vals":{"MAIL_FROM":"a@b.co"}}`},
		{"unknown setting", `{"values":{"NOPE":"1"}}`},
		{"read-only setting", `{"values":{"HTTP_ADDR":":1"}}`},
		{"database url", `{"values":{"DATABASE_URL":"postgres://x"}}`},
		{"invalid value", `{"values":{"SMTP_PORT":"99999"}}`},
		{"password but no key configured", `{"values":{"SMTP_PASSWORD":"x"}}`},
	}
	for _, c := range cases {
		rec := do(r, "PUT", "/api/v1/settings", c.body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d (%s), want 400", c.name, rec.Code, rec.Body.String())
			continue
		}
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(rec.Body.Bytes(), &e) != nil || e.Error == "" {
			t.Errorf("%s: want a JSON {error} body, got %s", c.name, rec.Body.String())
		}
	}
}

func TestUpdateSettings_OversizedBody(t *testing.T) {
	big := `{"values":{"MAIL_FROM":"` + strings.Repeat("a", 70<<10) + `"}}`
	if rec := do(settingsRouter(), "PUT", "/api/v1/settings", big); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestTestSMTP_ValidationAndConnectionFailure(t *testing.T) {
	r := settingsRouter()

	for name, body := range map[string]string{
		"missing to":    `{}`,
		"bad to":        `{"to":"nope"}`,
		"header inject": `{"to":"a@b.co\r\nBcc: x@y.z"}`,
		"bad port":      `{"to":"a@b.co","port":"abc"}`,
		"bad tls mode":  `{"to":"a@b.co","tls_mode":"ssl"}`,
		"unknown field": `{"to":"a@b.co","hots":"x"}`,
	} {
		if rec := do(r, "POST", "/api/v1/settings/smtp/test", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d (%s), want 400", name, rec.Code, rec.Body.String())
		}
	}

	// A valid request to a port nothing listens on is a delivery failure (502),
	// not a client error, and the reason is reported.
	rec := do(r, "POST", "/api/v1/settings/smtp/test", `{"to":"a@b.co","host":"127.0.0.1","port":"1"}`)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "test email failed") {
		t.Fatalf("status = %d body = %s, want 502 with reason", rec.Code, rec.Body.String())
	}
}

func TestGetStats_ParameterValidation(t *testing.T) {
	r := NewRouter(NewHandler(&fakeMailer{}, nil, nil))

	for _, q := range []string{
		"days=0", "days=-3", "days=91", "days=abc", "days=1.5",
		"tz=Mars/Base", "tz=Local", "tz=%20", "tz=UTC%0d%0aX-Evil:1",
	} {
		rec := do(r, "GET", "/api/v1/stats?"+q, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("?%s: status = %d (%s), want 400", q, rec.Code, rec.Body.String())
		}
	}

	// Valid parameters get past validation; with no database configured they
	// reach the "store not configured" error instead of a 400.
	for _, q := range []string{"", "days=1", "days=90", "tz=UTC", "tz=Asia/Dhaka&days=30"} {
		rec := do(r, "GET", "/api/v1/stats?"+q, "")
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("?%s: status = %d, want 500 (no store)", q, rec.Code)
		}
	}
}

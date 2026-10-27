package settings

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"sendmail/internal/config"
	"sendmail/internal/mailer"
	"sendmail/internal/migrations"
	"sendmail/internal/store"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var testKey = []byte("0123456789abcdef0123456789abcdef") // 32 bytes

func baseEnv() config.Config {
	return config.Config{
		SMTPHost: "localhost", SMTPPort: "1025", SMTPTLSMode: "auto",
		MailFrom: "env@example.com", MaxJobRetries: 3, WorkerCount: 3, SMTPPoolSize: 3,
		DomainCheckEnabled: true, HTTPAddr: ":8080", SMTPPassword: "env-password",
	}
}

// ---- pure unit tests (no database) ----

func TestCrypto_RoundTripAndFailures(t *testing.T) {
	enc, err := encrypt(testKey, "hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(enc, "hunter2") || !strings.HasPrefix(enc, encPrefix) {
		t.Fatalf("ciphertext leaks plaintext or lacks prefix: %q", enc)
	}
	if got, err := decrypt(testKey, enc); err != nil || got != "hunter2" {
		t.Fatalf("round trip: %q, %v", got, err)
	}

	enc2, _ := encrypt(testKey, "hunter2")
	if enc == enc2 {
		t.Fatal("same plaintext must encrypt differently each time (random nonce)")
	}

	wrong := []byte("ffffffffffffffffffffffffffffffff")
	if _, err := decrypt(wrong, enc); err == nil {
		t.Fatal("decrypting with the wrong key must fail")
	}
	if _, err := decrypt(testKey, enc[:len(enc)-4]); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
	if _, err := encrypt(nil, "x"); !errors.Is(err, ErrNoEncryptionKey) {
		t.Fatalf("no key: err = %v", err)
	}
}

func TestParseKey(t *testing.T) {
	if k, err := ParseKey(""); err != nil || k != nil {
		t.Fatalf("empty key should mean 'not configured': %v %v", k, err)
	}
	if _, err := ParseKey(base64.StdEncoding.EncodeToString(testKey)); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseKey("not base64!!"); err == nil {
		t.Fatal("expected base64 error")
	}
	if _, err := ParseKey(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("expected length error")
	}
}

func TestParsers(t *testing.T) {
	cases := []struct {
		key, in, want string
		ok            bool
	}{
		{"MAIL_FROM", " me@example.com ", "me@example.com", true},
		{"MAIL_FROM", "Me <me@example.com>", "", false}, // display names rejected
		{"MAIL_FROM", "not-an-email", "", false},
		{"MAIL_FROM", "a@example.com\r\nBcc: x@y.z", "", false}, // header injection
		{"SMTP_HOST", "smtp.example.com", "smtp.example.com", true},
		{"SMTP_HOST", "smtp.example.com/evil", "", false},
		{"SMTP_HOST", "", "", false},
		{"SMTP_PORT", "587", "587", true},
		{"SMTP_PORT", "0", "", false},
		{"SMTP_PORT", "70000", "", false},
		{"SMTP_PORT", "abc", "", false},
		{"SMTP_TLS_MODE", "STARTTLS", "starttls", true},
		{"SMTP_TLS_MODE", "ssl", "", false},
		{"SMTP_USERNAME", "", "", true}, // empty = no auth
		{"SMTP_PASSWORD", "  has spaces  ", "  has spaces  ", true},
		{"SMTP_PASSWORD", "line\nbreak", "", false},
		{"DOMAIN_CHECK_ENABLED", "FALSE", "false", true},
		{"DOMAIN_CHECK_ENABLED", "maybe", "", false},
		{"MAX_JOB_RETRIES", "5", "5", true},
		{"MAX_JOB_RETRIES", "0", "", false},
		{"MAX_JOB_RETRIES", "99", "", false},
		{"WORKER_COUNT", "10", "10", true},
		{"WORKER_COUNT", "0", "", false},
		{"JOB_POLL_INTERVAL", "1s", "1s", true},
		{"JOB_POLL_INTERVAL", "1ms", "", false},
		{"JOB_POLL_INTERVAL", "soon", "", false},
	}
	for _, c := range cases {
		d, ok := findDef(c.key)
		if !ok {
			t.Fatalf("no def for %s", c.key)
		}
		got, err := d.parse(c.in)
		if (err == nil) != c.ok || (c.ok && got != c.want) {
			t.Errorf("%s(%q) = %q, %v; want %q ok=%v", c.key, c.in, got, err, c.want, c.ok)
		}
	}
}

func TestCatalog_NoDatabaseURLAndReadOnlyHaveNoSetter(t *testing.T) {
	for _, d := range catalog {
		if d.key == "DATABASE_URL" {
			t.Fatal("DATABASE_URL must never be exposed")
		}
		if d.editable() && (d.parse == nil || d.set == nil) {
			t.Errorf("%s is editable but lacks parse/set", d.key)
		}
		if d.get == nil {
			t.Errorf("%s lacks get", d.key)
		}
	}
}

// ---- service tests (need a database) ----

// openStore connects to TEST_DATABASE_URL, or skips. It also refuses to run
// against the "public" schema of a database not named like a test database,
// because these tests wipe app_settings.
func openStore(t *testing.T) *store.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	var schema, dbname string
	if err := db.QueryRow(`SELECT current_schema(), current_database()`).Scan(&schema, &dbname); err != nil {
		t.Fatal(err)
	}
	if schema == "public" && !strings.Contains(dbname, "test") {
		t.Skipf("refusing to run: TEST_DATABASE_URL points at the public schema of %q; use a scratch schema or a *test* database", dbname)
	}

	st := store.New(db)
	if _, err := migrations.Up(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	wipe := func() { db.Exec(`DELETE FROM app_settings`) }
	wipe()
	t.Cleanup(wipe)
	return st
}

type harness struct {
	svc     *Service
	st      *store.Store
	applied []config.Config
	checks  []mailer.ConnConfig
	checkFn func(mailer.ConnConfig) error
	mu      sync.Mutex
}

func newHarness(t *testing.T, key []byte) *harness {
	t.Helper()
	h := &harness{st: openStore(t)}
	h.svc = New(h.st, baseEnv(), key, func(c config.Config) {
		h.mu.Lock()
		h.applied = append(h.applied, c)
		h.mu.Unlock()
	})
	h.svc.checkSMTP = func(_ context.Context, c mailer.ConnConfig) error {
		h.checks = append(h.checks, c)
		if h.checkFn != nil {
			return h.checkFn(c)
		}
		return nil
	}
	if _, err := h.svc.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *harness) item(t *testing.T, key string) Item {
	t.Helper()
	for _, it := range h.svc.List() {
		if it.Key == key {
			return it
		}
	}
	t.Fatalf("no item %s", key)
	return Item{}
}

func isInput(err error) bool {
	var in *InputError
	return errors.As(err, &in)
}

func TestService_ListShowsEnvDefaultsAndHidesSecrets(t *testing.T) {
	h := newHarness(t, testKey)

	from := h.item(t, "MAIL_FROM")
	if from.Value != "env@example.com" || from.Source != "env" || !from.Editable || from.Apply != ApplyInstant {
		t.Fatalf("MAIL_FROM item: %+v", from)
	}
	pw := h.item(t, "SMTP_PASSWORD")
	if pw.Value != "" || !pw.HasValue || !pw.Secret {
		t.Fatalf("password must be hidden but flagged as set: %+v", pw)
	}
	if addr := h.item(t, "HTTP_ADDR"); addr.Editable || addr.Apply != ApplyReadOnly {
		t.Fatalf("HTTP_ADDR must be read-only: %+v", addr)
	}
	for _, it := range h.svc.List() {
		if strings.Contains(it.Value, "env-password") {
			t.Fatalf("secret leaked in %s", it.Key)
		}
	}
}

func TestService_UpdateInstantSettingAppliesAndPersists(t *testing.T) {
	h := newHarness(t, testKey)
	ctx := context.Background()

	items, err := h.svc.Update(ctx, Change{Values: map[string]string{"MAIL_FROM": "new@example.com", "MAX_JOB_RETRIES": "5"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.applied) != 1 || h.applied[0].MailFrom != "new@example.com" || h.applied[0].MaxJobRetries != 5 {
		t.Fatalf("applier not called with new config: %+v", h.applied)
	}
	if len(h.checks) != 0 {
		t.Fatal("non-SMTP change must not trigger an SMTP check")
	}
	for _, it := range items {
		if it.Key == "MAIL_FROM" && (it.Source != "override" || it.Value != "new@example.com" || it.UpdatedAt == nil) {
			t.Fatalf("MAIL_FROM after update: %+v", it)
		}
	}

	// A fresh service (simulating a restart) sees the saved override.
	fresh := New(h.st, baseEnv(), testKey, nil)
	cfg, err := fresh.Load(ctx)
	if err != nil || cfg.MailFrom != "new@example.com" || cfg.MaxJobRetries != 5 {
		t.Fatalf("after reload: %+v, %v", cfg, err)
	}
}

func TestService_SMTPChangeIsCheckedAndFailureSavesNothing(t *testing.T) {
	h := newHarness(t, testKey)
	ctx := context.Background()
	h.checkFn = func(c mailer.ConnConfig) error { return errors.New("auth rejected") }

	_, err := h.svc.Update(ctx, Change{Values: map[string]string{"SMTP_HOST": "smtp.example.com", "SMTP_PORT": "587"}})
	if err == nil || !isInput(err) || !strings.Contains(err.Error(), "auth rejected") {
		t.Fatalf("err = %v, want an input error carrying the SMTP reason", err)
	}
	if len(h.checks) != 1 || h.checks[0].Host != "smtp.example.com" || h.checks[0].Port != "587" {
		t.Fatalf("check should try the NEW values: %+v", h.checks)
	}
	if len(h.applied) != 0 {
		t.Fatal("nothing may be applied after a failed check")
	}
	if got := h.item(t, "SMTP_HOST"); got.Value != "localhost" || got.Source != "env" {
		t.Fatalf("SMTP_HOST must be unchanged: %+v", got)
	}
	if rows, _ := h.st.GetSettings(ctx); len(rows) != 0 {
		t.Fatalf("nothing may be stored after a failed check: %v", rows)
	}

	// Same change, server now OK: saved and applied.
	h.checkFn = nil
	if _, err := h.svc.Update(ctx, Change{Values: map[string]string{"SMTP_HOST": "smtp.example.com", "SMTP_PORT": "587"}}); err != nil {
		t.Fatal(err)
	}
	if len(h.applied) != 1 || h.applied[0].SMTPHost != "smtp.example.com" {
		t.Fatalf("applied = %+v", h.applied)
	}
}

func TestService_PasswordIsEncryptedAtRestAndNeverReturned(t *testing.T) {
	h := newHarness(t, testKey)
	ctx := context.Background()

	items, err := h.svc.Update(ctx, Change{Values: map[string]string{"SMTP_PASSWORD": "s3cret-pw"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.checks) != 1 || h.checks[0].Password != "s3cret-pw" {
		t.Fatalf("check must use the new password: %+v", h.checks)
	}
	for _, it := range items {
		if strings.Contains(it.Value, "s3cret-pw") {
			t.Fatal("password leaked through the API model")
		}
	}

	rows, _ := h.st.GetSettings(ctx)
	row := rows["SMTP_PASSWORD"]
	if !row.Secret || !strings.HasPrefix(row.Value, encPrefix) || strings.Contains(row.Value, "s3cret-pw") {
		t.Fatalf("password must be stored encrypted: %+v", row)
	}

	fresh := New(h.st, baseEnv(), testKey, nil)
	if cfg, err := fresh.Load(ctx); err != nil || cfg.SMTPPassword != "s3cret-pw" {
		t.Fatalf("reload should decrypt: %q, %v", cfg.SMTPPassword, err)
	}

	// Wrong key on a later start: the unreadable override is skipped, env value used.
	wrong := New(h.st, baseEnv(), []byte("ffffffffffffffffffffffffffffffff"), nil)
	if cfg, err := wrong.Load(ctx); err != nil || cfg.SMTPPassword != "env-password" {
		t.Fatalf("wrong key should fall back to env: %q, %v", cfg.SMTPPassword, err)
	}
}

func TestService_PasswordWithoutKeyIsRejected(t *testing.T) {
	h := newHarness(t, nil)

	_, err := h.svc.Update(context.Background(), Change{Values: map[string]string{"SMTP_PASSWORD": "x"}})
	if !isInput(err) || !strings.Contains(err.Error(), "SETTINGS_ENCRYPTION_KEY") {
		t.Fatalf("err = %v", err)
	}
	if len(h.checks) != 0 || len(h.applied) != 0 {
		t.Fatal("rejected change must have no effects")
	}
}

func TestService_RejectionsAreAllOrNothing(t *testing.T) {
	h := newHarness(t, testKey)
	ctx := context.Background()

	bad := []Change{
		{Values: map[string]string{"HTTP_ADDR": ":9999"}},                                // read-only
		{Values: map[string]string{"DATABASE_URL": "postgres://x"}},                      // not exposed
		{Values: map[string]string{"NOPE": "1"}},                                         // unknown
		{Values: map[string]string{"MAX_JOB_RETRIES": "0"}},                              // invalid value
		{Values: map[string]string{"MAIL_FROM": "ok@example.com", "SMTP_PORT": "99999"}}, // one good, one bad
		{Reset: []string{"HTTP_ADDR"}},                                                   // reset read-only
		{Values: map[string]string{"MAIL_FROM": "a@b.co"}, Reset: []string{"MAIL_FROM"}}, // contradictory
	}
	for i, ch := range bad {
		if _, err := h.svc.Update(ctx, ch); !isInput(err) {
			t.Errorf("case %d: err = %v, want input error", i, err)
		}
	}
	if len(h.applied) != 0 {
		t.Fatal("nothing may be applied")
	}
	if rows, _ := h.st.GetSettings(ctx); len(rows) != 0 {
		t.Fatalf("nothing may be stored (good value in a bad batch must not leak in): %v", rows)
	}
	if got := h.item(t, "MAIL_FROM"); got.Value != "env@example.com" {
		t.Fatalf("MAIL_FROM changed despite rejection: %+v", got)
	}
}

func TestService_ResetRestoresEnvValue(t *testing.T) {
	h := newHarness(t, testKey)
	ctx := context.Background()

	if _, err := h.svc.Update(ctx, Change{Values: map[string]string{"MAIL_FROM": "temp@example.com"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Update(ctx, Change{Reset: []string{"MAIL_FROM"}}); err != nil {
		t.Fatal(err)
	}
	if got := h.item(t, "MAIL_FROM"); got.Value != "env@example.com" || got.Source != "env" {
		t.Fatalf("after reset: %+v", got)
	}
	if last := h.applied[len(h.applied)-1]; last.MailFrom != "env@example.com" {
		t.Fatalf("reset must be applied too: %+v", last)
	}
	if rows, _ := h.st.GetSettings(ctx); len(rows) != 0 {
		t.Fatalf("override row should be deleted: %v", rows)
	}
}

func TestService_RestartOnlySettingsAreFlaggedNotApplied(t *testing.T) {
	h := newHarness(t, testKey)
	ctx := context.Background()

	if _, err := h.svc.Update(ctx, Change{Values: map[string]string{"WORKER_COUNT": "8"}}); err != nil {
		t.Fatal(err)
	}
	it := h.item(t, "WORKER_COUNT")
	if it.Value != "8" || !it.RestartRequired || it.Apply != ApplyRestart {
		t.Fatalf("WORKER_COUNT after save: %+v", it)
	}
	if h.item(t, "MAIL_FROM").RestartRequired {
		t.Fatal("instant settings must never be flagged restart-required")
	}

	// After a "restart" (fresh Load) the flag clears: running == effective.
	fresh := New(h.st, baseEnv(), testKey, nil)
	cfg, _ := fresh.Load(ctx)
	if cfg.WorkerCount != 8 {
		t.Fatalf("worker count after restart = %d", cfg.WorkerCount)
	}
	for _, it := range fresh.List() {
		if it.RestartRequired {
			t.Fatalf("%s still flagged after restart", it.Key)
		}
	}
}

func TestService_NoOpValueCreatesNoOverride(t *testing.T) {
	h := newHarness(t, testKey)
	ctx := context.Background()

	if _, err := h.svc.Update(ctx, Change{Values: map[string]string{"MAIL_FROM": "env@example.com", "SMTP_PORT": "1025"}}); err != nil {
		t.Fatal(err)
	}
	if len(h.applied) != 0 || len(h.checks) != 0 {
		t.Fatal("unchanged values must not apply or check anything")
	}
	if rows, _ := h.st.GetSettings(ctx); len(rows) != 0 {
		t.Fatalf("no override expected: %v", rows)
	}
}

func TestService_SendTestValidatesInput(t *testing.T) {
	h := newHarness(t, testKey)

	for _, req := range []TestRequest{
		{To: ""},
		{To: "not-an-email"},
		{To: "a@example.com\r\nBcc: x@y.z"},
		{To: "a@example.com", Port: ptr("99999")},
		{To: "a@example.com", TLSMode: ptr("ssl")},
		{To: "a@example.com", From: ptr("bad from")},
	} {
		if _, err := h.svc.SendTest(context.Background(), req); !isInput(err) {
			t.Errorf("%+v: err = %v, want input error", req, err)
		}
	}
}

func ptr(s string) *string { return &s }

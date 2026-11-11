package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sendmail/internal/importer"
	"sendmail/internal/migrations"
	"sendmail/internal/store"
	"sendmail/internal/worker"
)

// recordingMailer remembers every email and can refuse some addresses.
type recordingMailer struct {
	mu     sync.Mutex
	sent   map[string]string // to -> subject
	reject map[string]bool   // to -> answer 550
}

func (m *recordingMailer) Send(to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reject[to] {
		return fmt.Errorf("rcpt to %q: %w", to, &textproto.Error{Code: 550, Msg: "no such user"})
	}
	m.sent[to] = subject
	return nil
}

func (m *recordingMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

type flowEnv struct {
	t      *testing.T
	st     *store.Store
	router http.Handler
	imp    *importer.Worker
	gen    *worker.Generator
	mail   *recordingMailer
}

// newFlow wires the real store, import worker, generator and handler. The
// workers are driven by hand (RunOnce) so each step is deterministic; the
// sending pool is started only where a test needs it.
func newFlow(t *testing.T) *flowEnv {
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
	db.QueryRow(`SELECT current_schema(), current_database()`).Scan(&schema, &dbname)
	if schema == "public" && !strings.Contains(dbname, "test") {
		t.Skipf("refusing to run on the public schema of %q", dbname)
	}
	if _, err := migrations.Up(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	// Start from an empty queue so other tests' leftovers are not picked up.
	db.Exec(`DELETE FROM email_batches`)

	st := store.New(db)
	files := importer.Files{Dir: t.TempDir()}
	env := &flowEnv{t: t, st: st, mail: &recordingMailer{sent: map[string]string{}, reject: map[string]bool{}}}
	env.imp = importer.NewWorker(st, files, nil, 0)
	env.gen = worker.NewGenerator(st, 0, nil)
	env.gen.ChunkSize = 2
	env.router = NewRouter(NewHandler(env.mail, st, nil).WithImports(files, env.imp, env.gen))
	return env
}

func (e *flowEnv) do(method, path, body string) (int, map[string]any) {
	e.t.Helper()
	rec := do(e.router, method, path, body)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (e *flowEnv) upload(route, file string) int64 {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "samples", file))
	if err != nil {
		e.t.Fatal(err)
	}
	rec := postFormTo(e.router, route, "file", file, data)
	if rec.Code != http.StatusAccepted {
		e.t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	var resp uploadResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Status != "importing" || resp.Filename != file {
		e.t.Fatalf("upload response = %+v", resp)
	}
	if worked, err := e.imp.RunOnce(context.Background()); !worked || err != nil {
		e.t.Fatalf("import: %v %v", worked, err)
	}
	return resp.BatchID
}

func (e *flowEnv) waitFor(id int64, statuses ...string) map[string]any {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, b := e.do("GET", fmt.Sprintf("/api/v1/batches/%d", id), "")
		for _, s := range statuses {
			if b["status"] == s {
				return b
			}
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("batch %d stuck in %v, want %v", id, b["status"], statuses)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Verify a file, look at its problems, then send that same batch later:
// upload -> background import -> issues -> sample -> send -> generate ->
// deliver -> results, all through the HTTP API.
func TestFlow_VerifyThenSend(t *testing.T) {
	e := newFlow(t)
	id := e.upload("/api/v1/verify", "9_mixed_everything.xlsx")

	code, b := e.do("GET", fmt.Sprintf("/api/v1/verify/%d", id), "")
	if code != 200 || b["status"] != "completed" || b["type"] != "verification" || b["total_rows"].(float64) != 8 ||
		b["valid_count"].(float64) != 4 || b["invalid_count"].(float64) != 4 {
		t.Fatalf("verify summary = %d %v", code, b)
	}

	_, issues := e.do("GET", fmt.Sprintf("/api/v1/verify/%d/issues?limit=3", id), "")
	items := issues["items"].([]any)
	if len(items) != 3 || issues["next_after"] == nil || items[0].(map[string]any)["row"].(float64) != 3 {
		t.Fatalf("issues page 1 = %v", issues)
	}
	_, more := e.do("GET", fmt.Sprintf("/api/v1/verify/%d/issues?limit=3&after=%.0f", id, issues["next_after"].(float64)), "")
	if len(more["items"].([]any)) != 1 || more["next_after"] != nil {
		t.Fatalf("issues page 2 = %v", more)
	}
	_, byReason := e.do("GET", fmt.Sprintf("/api/v1/verify/%d/issues?reason=invalid_email", id), "")
	if len(byReason["items"].([]any)) != 2 {
		t.Fatalf("invalid_email rows = %v", byReason)
	}
	_, three := e.do("GET", fmt.Sprintf("/api/v1/verify/%d/issues?reason=invalid_email,empty_name", id), "")
	if len(three["items"].([]any)) != 3 {
		t.Fatalf("two reasons = %v", three)
	}
	for _, bad := range []string{"?status=sent", "?reason=duplicate", "?status=unverified&reason=bad_domain", "?after=-1", "?limit=0"} {
		if code, _ := e.do("GET", fmt.Sprintf("/api/v1/verify/%d/issues%s", id, bad), ""); code != 400 {
			t.Errorf("issues%s: status %d, want 400", bad, code)
		}
	}
	_, sample := e.do("GET", fmt.Sprintf("/api/v1/batches/%d/sample?limit=2", id), "")
	if s := sample["items"].([]any); len(s) != 2 || s[0].(map[string]any)["validation_status"] != "valid" {
		t.Fatalf("sample = %v", sample)
	}

	// Send it.
	path := fmt.Sprintf("/api/v1/batches/%d/send", id)
	if code, body := e.do("POST", path, `{"subject":"Hi {{.Phone}}","body":"x"}`); code != 400 {
		t.Fatalf("unknown template field: %d %v", code, body)
	}
	code, b = e.do("POST", path, `{"subject":"Hi {{.Name}}","body":"Hello {{.Name}}"}`)
	if code != 202 || b["status"] != "generating_jobs" || b["type"] != "bulk_send" {
		t.Fatalf("send = %d %v", code, b)
	}
	if code, _ := e.do("POST", path, `{"subject":"again","body":"again"}`); code != 409 {
		t.Fatalf("double send: %d, want 409", code)
	}

	if worked, err := e.gen.RunOnce(context.Background()); !worked || err != nil {
		t.Fatalf("generate: %v %v", worked, err)
	}
	e.waitFor(id, "queued")

	// One recipient's server refuses the email for good (550). This sample's
	// first address appears twice (rows 2 and 8): duplicates are not flagged,
	// so both rows get an email, and both are refused.
	_, first := e.do("GET", fmt.Sprintf("/api/v1/batches/%d/sample?limit=1", id), "")
	refused := first["items"].([]any)[0].(map[string]any)["email"].(string)
	e.mail.reject[refused] = true

	ctx, cancel := context.WithCancel(context.Background())
	pool := worker.NewPool(3, 10*time.Millisecond, e.st, e.mail)
	pool.Start(ctx)
	defer func() { cancel(); pool.Stop() }()

	b = e.waitFor(id, "completed_with_errors")
	if b["sent_count"].(float64) != 2 || b["failed_count"].(float64) != 2 || b["pending_count"].(float64) != 0 {
		t.Fatalf("after sending = %v", b)
	}
	for to, subject := range e.mail.sent {
		if !strings.HasPrefix(subject, "Hi ") || subject == "Hi " {
			t.Errorf("email to %s has subject %q, want it built from the template", to, subject)
		}
	}

	_, failed := e.do("GET", fmt.Sprintf("/api/v1/batches/%d/emails?status=failed", id), "")
	fi := failed["items"].([]any)
	if len(fi) != 2 || fi[1].(map[string]any)["email"] != refused || !strings.Contains(fi[0].(map[string]any)["last_error"].(string), "550") {
		t.Fatalf("failed emails = %v", failed)
	}
	if fi[0].(map[string]any)["subject"] == "" || fi[0].(map[string]any)["attempts"].(float64) != 1 {
		t.Fatalf("a 550 is not retried, and the list shows the built subject: %v", fi[0])
	}

	// The recipient's server is fixed; retry the batch.
	delete(e.mail.reject, refused)
	code, retried := e.do("POST", fmt.Sprintf("/api/v1/batches/%d/retry", id), "")
	if code != 200 || retried["retried"].(float64) != 2 {
		t.Fatalf("retry = %d %v", code, retried)
	}
	e.waitFor(id, "completed")
	if e.mail.count() != 3 {
		t.Fatalf("mailer saw %d distinct addresses, want 3 (4 emails, one address twice)", e.mail.count())
	}

	_, list := e.do("GET", "/api/v1/batches?type=bulk_send", "")
	if list["total"].(float64) != 1 || list["items"].([]any)[0].(map[string]any)["id"].(float64) != float64(id) {
		t.Fatalf("bulk list = %v", list)
	}
	_, all := e.do("GET", fmt.Sprintf("/api/v1/batches/%d/emails?limit=2", id), "")
	if len(all["items"].([]any)) != 2 || all["next_after"] == nil {
		t.Fatalf("emails page = %v", all)
	}
}

func TestFlow_CancelAndConflicts(t *testing.T) {
	e := newFlow(t)
	id := e.upload("/api/v1/batches", "10_large_mixed_1000.xlsx")
	_, b := e.do("GET", fmt.Sprintf("/api/v1/batches/%d", id), "")
	if b["status"] != "draft" || b["type"] != "bulk_send" {
		t.Fatalf("after import = %v", b)
	}
	// Nothing to cancel yet.
	if code, _ := e.do("POST", fmt.Sprintf("/api/v1/batches/%d/cancel", id), ""); code != 409 {
		t.Fatalf("cancel a draft: %d", code)
	}
	e.do("POST", fmt.Sprintf("/api/v1/batches/%d/send", id), `{"subject":"s","body":"b"}`)
	e.gen.RunOnce(context.Background())
	_, b = e.do("GET", fmt.Sprintf("/api/v1/batches/%d", id), "")
	pending := b["pending_count"].(float64)
	if b["status"] != "queued" || pending == 0 {
		t.Fatalf("queued = %v", b)
	}

	code, c := e.do("POST", fmt.Sprintf("/api/v1/batches/%d/cancel", id), "")
	if code != 200 || c["cancelled"].(float64) != pending {
		t.Fatalf("cancel = %d %v, want %v cancelled", code, c, pending)
	}
	if code, _ := e.do("POST", fmt.Sprintf("/api/v1/batches/%d/cancel", id), ""); code != 409 {
		t.Fatalf("second cancel: %d", code)
	}
	if code, _ := e.do("POST", fmt.Sprintf("/api/v1/batches/%d/retry", id), ""); code != 409 {
		t.Fatalf("retry after cancel: %d", code)
	}
	for path, want := range map[string]int{
		"/api/v1/batches/999999999":            404,
		"/api/v1/batches/abc":                  400,
		"/api/v1/batches?status=nope":          400,
		"/api/v1/batches?type=nope":            400,
		"/api/v1/batches/999999999/issues":     404,
		"/api/v1/batches/999999999/emails":     404,
		"/api/v1/batches/1/emails?status=nope": 400,
	} {
		if code, _ := e.do("GET", path, ""); code != want {
			t.Errorf("GET %s = %d, want %d", path, code, want)
		}
	}
	if code, _ := e.do("POST", "/api/v1/batches/999999999/send", `{"subject":"s","body":"b"}`); code != 404 {
		t.Errorf("send unknown: %d", code)
	}
	if code, _ := e.do("POST", "/api/v1/jobs/999999999/retry", ""); code != 404 {
		t.Errorf("retry unknown job: %d", code)
	}
}

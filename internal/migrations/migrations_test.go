package migrations

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// These checks run without a database and catch the common authoring mistakes.
func TestEmbeddedMigrationsAreWellFormed(t *testing.T) {
	entries, err := fs.ReadDir(files, "sql")
	if err != nil {
		t.Fatal(err)
	}
	name := regexp.MustCompile(`^(\d{5})_[a-z0-9_]+\.sql$`)
	for i, e := range entries {
		m := name.FindStringSubmatch(e.Name())
		if m == nil {
			t.Errorf("%s: name must look like 00001_describe_change.sql", e.Name())
			continue
		}
		if n, _ := strconv.Atoi(m[1]); n != i+1 {
			t.Errorf("%s: numbers must run 1,2,3... without gaps or repeats (expected %d)", e.Name(), i+1)
		}
		body, _ := fs.ReadFile(files, "sql/"+e.Name())
		text := string(body)
		up, down := strings.Index(text, "-- +goose Up"), strings.Index(text, "-- +goose Down")
		if up < 0 || down < 0 || down < up {
			t.Errorf("%s: needs '-- +goose Up' followed by '-- +goose Down'", e.Name())
		}
		if strings.TrimSpace(text[down+len("-- +goose Down"):]) == "" {
			t.Errorf("%s: the Down section is empty, so this migration cannot be rolled back", e.Name())
		}
	}
	if len(entries) == 0 {
		t.Fatal("no migrations embedded")
	}
}

// openTestDB connects to TEST_DATABASE_URL, or skips. Because this test rolls
// migrations back (dropping tables), it refuses to run against the "public"
// schema of a database not clearly named as a test database.
func openTestDB(t *testing.T) *sql.DB {
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
	return db
}

func tables(t *testing.T, db *sql.DB) string {
	t.Helper()
	var s sql.NullString
	err := db.QueryRow(`SELECT string_agg(table_name, ',' ORDER BY table_name)
		FROM information_schema.tables WHERE table_schema = current_schema() AND table_name <> 'goose_db_version'`).Scan(&s)
	if err != nil {
		t.Fatal(err)
	}
	return s.String
}

func TestUpDownUp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	p, err := provider(db)
	if err != nil {
		t.Fatal(err)
	}
	// Start from zero so the test is independent of earlier runs.
	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if got := tables(t, db); got != "" {
		t.Fatalf("after rolling everything back, tables left: %q", got)
	}

	res, err := Up(ctx, db)
	if err != nil || len(res) != len(p.ListSources()) {
		t.Fatalf("Up applied %d of %d: %v", len(res), len(p.ListSources()), err)
	}
	if got := tables(t, db); got != "app_settings,batches,email_jobs" {
		t.Fatalf("tables after Up = %q", got)
	}
	if again, err := Up(ctx, db); err != nil || len(again) != 0 {
		t.Fatalf("a second Up must do nothing: %d, %v", len(again), err)
	}
	if pending, err := Pending(ctx, db); err != nil || pending {
		t.Fatalf("Pending = %v, %v; want false", pending, err)
	}

	// Every Down must really undo its Up: roll back one step at a time, then re-apply.
	for range p.ListSources() {
		if _, err := Down(ctx, db); err != nil {
			t.Fatalf("Down: %v", err)
		}
	}
	if got := tables(t, db); got != "" {
		t.Fatalf("tables left after rolling back every migration: %q", got)
	}
	if pending, _ := Pending(ctx, db); !pending {
		t.Fatal("Pending should be true after rolling back")
	}
	if _, err := Up(ctx, db); err != nil {
		t.Fatalf("re-applying after a full rollback failed: %v", err)
	}
}

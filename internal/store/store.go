// Package store manages all database operations: batches, their spreadsheet
// rows (recipients), the background import and job-generation tasks, the
// email delivery queue, and saved settings.
//
// Concept: repository pattern. The API and background workers don't write
// raw SQL; they talk to this Store. This keeps all SQL statements, transactions,
// and table schemas organized in one place.
//
// We use Go's built-in "database/sql" package with the pgx driver.
//
// The data model (migration 00006, decision 0013):
//
//	email_batches      one uploaded file: status, counters, templates, generation progress
//	email_recipients   one spreadsheet row (valid, unverified or invalid)
//	email_import_jobs  the background import of a batch's file (checkpoint + lease)
//	email_jobs         one email to deliver, created after the user clicks Send
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	// Register pgx as the database/sql driver. The blank identifier "_" imports
	// the package solely for its side-effects (driver registration with database/sql).
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Errors callers can test for with errors.Is.
var (
	// ErrNoJobs: there is no task or email ready to be claimed right now.
	ErrNoJobs = errors.New("no jobs available in queue")
	// ErrBatchNotFound: no batch with that ID.
	ErrBatchNotFound = errors.New("batch not found")
	// ErrBatchNotActive: the batch cannot be changed that way any more, e.g.
	// cancelling a finished batch or retrying a cancelled one.
	ErrBatchNotActive = errors.New("batch is not active")
	// ErrJobNotRetryable: the email does not exist or is not 'failed'.
	ErrJobNotRetryable = errors.New("job not found or not failed")
	// ErrNotSendable: the batch is not in a state that can be sent (still
	// importing, already sent, cancelled...).
	ErrNotSendable = errors.New("batch cannot be sent in its current state")
	// ErrNothingToSend: the batch has no valid or unverified rows.
	ErrNothingToSend = errors.New("batch has no recipients to send to")
	// ErrLeaseLost: another worker took over this task (our lease expired).
	ErrLeaseLost = errors.New("task lease lost to another worker")
)

// defaultMaxAttempts is used when no value is configured via SetMaxAttempts.
const defaultMaxAttempts = 3

// Store wraps the database connection.
type Store struct {
	db          *sql.DB
	maxAttempts atomic.Int64 // read by job generation while Settings may change it
}

// New creates a new Store backed by db.
func New(db *sql.DB) *Store {
	s := &Store{db: db}
	s.maxAttempts.Store(defaultMaxAttempts)
	return s
}

// DB exposes the connection pool (tests and maintenance helpers).
func (s *Store) DB() *sql.DB { return s.db }

// SetMaxAttempts sets how many delivery attempts newly generated emails get.
// Values below 1 are ignored.
func (s *Store) SetMaxAttempts(n int) {
	if n > 0 {
		s.maxAttempts.Store(int64(n))
	}
}

// inTx runs fn inside a transaction and commits it, or rolls back if fn fails.
func (s *Store) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() // no-op after Commit
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// ms turns a Go duration into milliseconds for SQL such as
// "NOW() - $1 * INTERVAL '1 millisecond'".
func ms(d time.Duration) int64 { return d.Milliseconds() }

// SettingRow is one stored settings override. For secret settings Value holds
// ciphertext; the store never interprets it.
type SettingRow struct {
	Value     string
	Secret    bool
	UpdatedAt time.Time
}

// GetSettings returns every stored settings override, keyed by setting name.
func (s *Store) GetSettings(ctx context.Context) (map[string]SettingRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value, secret, updated_at FROM app_settings`)
	if err != nil {
		return nil, fmt.Errorf("query settings: %w", err)
	}
	defer rows.Close()

	out := map[string]SettingRow{}
	for rows.Next() {
		var key string
		var r SettingRow
		if err := rows.Scan(&key, &r.Value, &r.Secret, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan setting: %w", err)
		}
		out[key] = r
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}
	return out, nil
}

// SaveSettings upserts the given overrides and deletes the listed keys, all in
// one transaction so a settings change is applied completely or not at all.
func (s *Store) SaveSettings(ctx context.Context, set map[string]SettingRow, remove []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for key, r := range set {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app_settings (key, value, secret, updated_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (key) DO UPDATE
			SET value = EXCLUDED.value, secret = EXCLUDED.secret, updated_at = NOW()
		`, key, r.Value, r.Secret); err != nil {
			return fmt.Errorf("save setting %s: %w", key, err)
		}
	}
	for _, key := range remove {
		if _, err := tx.ExecContext(ctx, `DELETE FROM app_settings WHERE key = $1`, key); err != nil {
			return fmt.Errorf("delete setting %s: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit settings: %w", err)
	}
	return nil
}

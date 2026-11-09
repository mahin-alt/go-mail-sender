package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// ErrUploadNotFound is returned for an unknown or expired upload.
var ErrUploadNotFound = errors.New("upload not found")

// copyChunkSize is how many rows are buffered before they are written with
// COPY. Big enough to be fast, small enough that memory stays flat.
const copyChunkSize = 5000

// Upload is one checked spreadsheet and its totals.
type Upload struct {
	ID              int64          `json:"upload_id"`
	Filename        string         `json:"filename"`
	Status          string         `json:"status"`
	TotalRows       int            `json:"total_rows"`
	ValidCount      int            `json:"valid_count"`      // will be sent: ok + unverified
	ProblemCount    int            `json:"problem_count"`    // will be skipped
	UnverifiedCount int            `json:"unverified_count"` // sent, domain not checked
	ProblemCounts   map[string]int `json:"problem_counts"`   // skipped rows per kind
	CreatedAt       time.Time      `json:"created_at"`
	ExpiresAt       time.Time      `json:"expires_at"`
}

// UploadRow is one stored spreadsheet row and its result.
type UploadRow struct {
	Row    int    `json:"row"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Kind   string `json:"kind"`
	Reason string `json:"reason,omitempty"`
}

// UploadMeta describes a file about to be imported.
type UploadMeta struct {
	Filename string
	SHA256   string
	// SkippedKinds are the kinds counted as problems; every kind is listed in
	// ProblemCounts even when its count is 0. Every other kind counts as valid,
	// and "unverified" additionally counts as unverified.
	SkippedKinds []string
}

// ImportUpload saves a checked file: it creates the upload, then calls fill,
// which must call add once per row (in row order). Rows are written to
// upload_rows with COPY in chunks while fill runs, so the whole file is never
// held in memory. Totals are counted on the way and saved on the upload.
//
// Everything happens in one transaction: if fill or the database fails, or
// ctx is cancelled (the client went away), nothing is saved.
//
// Concept: COPY is PostgreSQL's bulk loader, much faster than INSERT for many
// rows. database/sql has no COPY, so we borrow the underlying pgx connection.
func (s *Store) ImportUpload(ctx context.Context, meta UploadMeta, fill func(add func(UploadRow) error) error) (*Upload, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("get connection: %w", err)
	}
	defer conn.Close()

	var up *Upload
	err = conn.Raw(func(driverConn any) error {
		pc := driverConn.(*stdlib.Conn).Conn()
		tx, err := pc.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin transaction: %w", err)
		}
		defer tx.Rollback(ctx) // no-op after Commit

		var id int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO uploads (filename, file_sha256) VALUES ($1, $2) RETURNING id`,
			meta.Filename, meta.SHA256).Scan(&id); err != nil {
			return fmt.Errorf("insert upload: %w", err)
		}

		skipped := make(map[string]bool, len(meta.SkippedKinds))
		counts := make(map[string]int, len(meta.SkippedKinds))
		for _, k := range meta.SkippedKinds {
			skipped[k] = true
			counts[k] = 0
		}
		var total, valid, problems, unverified int

		buf := make([][]any, 0, copyChunkSize)
		flush := func() error {
			if len(buf) == 0 {
				return nil
			}
			_, err := tx.CopyFrom(ctx, pgx.Identifier{"upload_rows"},
				[]string{"upload_id", "row_num", "name", "email", "kind", "reason"},
				pgx.CopyFromRows(buf))
			buf = buf[:0]
			if err != nil {
				return fmt.Errorf("copy upload rows: %w", err)
			}
			return nil
		}
		add := func(r UploadRow) error {
			total++
			switch {
			case skipped[r.Kind]:
				problems++
				counts[r.Kind]++
			default:
				valid++
				if r.Kind == "unverified" {
					unverified++
				}
			}
			var reason *string
			if r.Reason != "" {
				reason = &r.Reason
			}
			buf = append(buf, []any{id, r.Row, r.Name, r.Email, r.Kind, reason})
			if len(buf) == copyChunkSize {
				return flush()
			}
			return nil
		}

		if err := fill(add); err != nil {
			return err
		}
		if err := flush(); err != nil {
			return err
		}

		countsJSON, err := json.Marshal(counts)
		if err != nil {
			return err
		}
		up = &Upload{ID: id, Filename: meta.Filename, ProblemCounts: counts,
			TotalRows: total, ValidCount: valid, ProblemCount: problems, UnverifiedCount: unverified}
		if err := tx.QueryRow(ctx, `
			UPDATE uploads
			SET status = 'ready', total_rows = $2, valid_count = $3, problem_count = $4,
			    unverified_count = $5, problem_counts = $6
			WHERE id = $1
			RETURNING status, created_at, expires_at`,
			id, total, valid, problems, unverified, countsJSON).
			Scan(&up.Status, &up.CreatedAt, &up.ExpiresAt); err != nil {
			return fmt.Errorf("save upload totals: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit upload: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return up, nil
}

// GetUpload returns an upload's totals. Expired uploads count as not found,
// even before the cleanup loop has deleted them.
func (s *Store) GetUpload(ctx context.Context, id int64) (*Upload, error) {
	var up Upload
	var counts []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, filename, status, total_rows, valid_count, problem_count,
		       unverified_count, problem_counts, created_at, expires_at
		FROM uploads
		WHERE id = $1 AND expires_at > NOW()`, id).
		Scan(&up.ID, &up.Filename, &up.Status, &up.TotalRows, &up.ValidCount, &up.ProblemCount,
			&up.UnverifiedCount, &counts, &up.CreatedAt, &up.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUploadNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get upload: %w", err)
	}
	if err := json.Unmarshal(counts, &up.ProblemCounts); err != nil {
		return nil, fmt.Errorf("decode problem counts: %w", err)
	}
	return &up, nil
}

// UploadRowFilter selects one page of an upload's rows.
type UploadRowFilter struct {
	Kinds  []string // required: only rows of these kinds
	Search string   // case-insensitive substring of name or email, "" = any
	After  int      // only rows after this spreadsheet row (0 = from the start)
	Limit  int
}

// ListUploadRows returns up to f.Limit rows of an upload in spreadsheet order.
//
// Concept: keyset ("cursor") paging. Instead of OFFSET (skip N rows, which
// gets slower the deeper you page), the next page starts "after row X", and
// the index on (upload_id, ..., row_num) jumps straight there.
func (s *Store) ListUploadRows(ctx context.Context, uploadID int64, f UploadRowFilter) ([]UploadRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT row_num, name, email, kind, COALESCE(reason, '')
		FROM upload_rows
		WHERE upload_id = $1
		  AND kind = ANY($2)
		  AND row_num > $3
		  AND ($4 = '' OR email ILIKE '%' || $4 || '%' OR name ILIKE '%' || $4 || '%')
		ORDER BY row_num
		LIMIT $5`,
		uploadID, f.Kinds, f.After, f.Search, f.Limit)
	if err != nil {
		return nil, fmt.Errorf("list upload rows: %w", err)
	}
	defer rows.Close()

	out := []UploadRow{} // non-nil so it encodes as [] not null
	for rows.Next() {
		var r UploadRow
		if err := rows.Scan(&r.Row, &r.Name, &r.Email, &r.Kind, &r.Reason); err != nil {
			return nil, fmt.Errorf("scan upload row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("upload rows iteration: %w", err)
	}
	return out, nil
}

// DeleteExpiredUploads removes uploads past their expiry time (their rows go
// with them) and returns how many were removed.
func (s *Store) DeleteExpiredUploads(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM uploads WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("delete expired uploads: %w", err)
	}
	return res.RowsAffected()
}

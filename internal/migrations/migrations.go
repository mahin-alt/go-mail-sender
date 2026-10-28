// Package migrations owns the database schema.
//
// Concept: a migration is a numbered SQL file describing ONE schema change.
// The tool (goose) remembers which numbers a database has already run in a
// table called goose_db_version, and only runs the missing ones, in order.
// That makes every environment (laptop, staging, production) converge on the
// same schema, and makes schema changes reviewable like any other code.
//
// The SQL files live in ./sql and are embedded in the binary, so deploying the
// server is a single file with nothing to copy alongside it.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed sql/*.sql
var files embed.FS

// provider builds a goose Provider for db.
//
// The session locker takes a PostgreSQL advisory lock while migrating, so if
// several server instances start at the same moment only one runs the
// migrations and the others wait, then find nothing left to do.
func provider(db *sql.DB) (*goose.Provider, error) {
	fsys, err := fs.Sub(files, "sql")
	if err != nil {
		return nil, err
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("create migration lock: %w", err)
	}
	return goose.NewProvider(goose.DialectPostgres, db, fsys, goose.WithSessionLocker(locker))
}

// Up applies every migration the database has not seen yet and returns the
// ones that ran (empty if already up to date).
func Up(ctx context.Context, db *sql.DB) ([]*goose.MigrationResult, error) {
	p, err := provider(db)
	if err != nil {
		return nil, err
	}
	return p.Up(ctx)
}

// UpByOne applies only the next pending migration. It returns ErrNoNextVersion
// (from goose) when nothing is pending.
func UpByOne(ctx context.Context, db *sql.DB) (*goose.MigrationResult, error) {
	p, err := provider(db)
	if err != nil {
		return nil, err
	}
	return p.UpByOne(ctx)
}

// UpTo applies pending migrations up to and including version.
func UpTo(ctx context.Context, db *sql.DB, version int64) ([]*goose.MigrationResult, error) {
	p, err := provider(db)
	if err != nil {
		return nil, err
	}
	return p.UpTo(ctx, version)
}

// DownTo rolls back migrations, newest first, until version is the newest one
// still applied. Version 0 rolls back everything.
func DownTo(ctx context.Context, db *sql.DB, version int64) ([]*goose.MigrationResult, error) {
	p, err := provider(db)
	if err != nil {
		return nil, err
	}
	return p.DownTo(ctx, version)
}

// Down rolls back the single most recently applied migration.
// Rolling back 00001 drops the batches and email_jobs tables and their data.
func Down(ctx context.Context, db *sql.DB) (*goose.MigrationResult, error) {
	p, err := provider(db)
	if err != nil {
		return nil, err
	}
	return p.Down(ctx)
}

// Status lists every known migration with whether (and when) it was applied.
func Status(ctx context.Context, db *sql.DB) ([]*goose.MigrationStatus, error) {
	p, err := provider(db)
	if err != nil {
		return nil, err
	}
	return p.Status(ctx)
}

// Pending reports whether the database is behind the migrations in this binary.
func Pending(ctx context.Context, db *sql.DB) (bool, error) {
	p, err := provider(db)
	if err != nil {
		return false, err
	}
	return p.HasPending(ctx)
}

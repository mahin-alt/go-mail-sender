// Command migrate manages the database schema by hand.
//
//	go run ./cmd/migrate status     show which migrations are applied
//	go run ./cmd/migrate up         apply all pending migrations
//	go run ./cmd/migrate down -yes  roll back the most recent migration
//	go run ./cmd/migrate create add_failed_at   make the next numbered migration file
//
// It reads DATABASE_URL from the environment / .env, like the server does.
// The server also applies pending migrations itself at startup (see
// AUTO_MIGRATE), so this tool is mainly for inspecting state, for deploys that
// run migrations as a separate step, and for rolling back.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sendmail/internal/config"
	"sendmail/internal/migrations"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: migrate <status|up|down|create> [-yes] [name]")
	}
	cmd := args[0]

	// "create" only writes a file, so it must work without a database.
	if cmd == "create" {
		if len(args) != 2 {
			return fmt.Errorf("usage: migrate create <name>   (example: migrate create add_failed_at)")
		}
		path, err := createMigration(migrationsDir, args[1])
		if err != nil {
			return err
		}
		fmt.Println("created", path)
		return nil
	}

	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	yes := fs.Bool("yes", false, "confirm a rollback (down)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	switch cmd {
	case "status":
		sts, err := migrations.Status(ctx, db)
		if err != nil {
			return err
		}
		fmt.Printf("%-8s %-45s %s\n", "VERSION", "NAME", "APPLIED")
		for _, s := range sts {
			applied := "pending"
			if s.State == "applied" {
				applied = s.AppliedAt.Format(time.RFC3339)
			}
			fmt.Printf("%-8d %-45s %s\n", s.Source.Version, s.Source.Path, applied)
		}
	case "up":
		res, err := migrations.Up(ctx, db)
		if err != nil {
			return err
		}
		if len(res) == 0 {
			fmt.Println("already up to date")
		}
		for _, r := range res {
			fmt.Printf("applied %s (%s)\n", r.Source.Path, r.Duration.Round(time.Millisecond))
		}
	case "down":
		if !*yes {
			return fmt.Errorf("down rolls back the latest migration and can DELETE DATA (rolling back 00001 drops every table); re-run with -yes to confirm")
		}
		r, err := migrations.Down(ctx, db)
		if err != nil {
			return err
		}
		fmt.Printf("rolled back %s\n", r.Source.Path)
	default:
		return fmt.Errorf("unknown command %q (use status, up, down or create)", cmd)
	}
	return nil
}

// migrationsDir is where the SQL files live, relative to the repository root.
const migrationsDir = "internal/migrations/sql"

var (
	fileNumber = regexp.MustCompile(`^(\d+)_.*\.sql$`)
	badNameRun = regexp.MustCompile(`[^a-z0-9]+`)
)

const migrationTemplate = `-- +goose Up
-- Write the change here. Prefer one focused change per migration.

-- +goose Down
-- Write the statements that undo the Up section exactly.
`

// createMigration writes an empty, correctly numbered migration file into dir
// and returns its path. Numbers are sequential (00004, 00005, ...), one more
// than the highest existing file.
func createMigration(dir, name string) (string, error) {
	slug := strings.Trim(badNameRun.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if slug == "" {
		return "", fmt.Errorf("migration name %q has no letters or digits", name)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read %s (run this from the repository root): %w", dir, err)
	}
	highest := 0
	for _, e := range entries {
		if m := fileNumber.FindStringSubmatch(e.Name()); m != nil {
			if n, _ := strconv.Atoi(m[1]); n > highest {
				highest = n
			}
		}
	}

	path := filepath.Join(dir, fmt.Sprintf("%05d_%s.sql", highest+1, slug))
	if err := os.WriteFile(path, []byte(migrationTemplate), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

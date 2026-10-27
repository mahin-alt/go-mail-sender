// Command server is the program's entry point.
//
// Convention: main packages live under cmd/<binary-name>/ and stay THIN.
// main's job is "wiring": build each piece, plug them together, start.
// All real logic lives in internal/ packages, where it can be tested.
//
// Systems wired together here:
//   1. config      - reads environment settings (.env)
//   2. database    - connects to PostgreSQL via database/sql (pgx driver)
//   3. store       - manages schema, batches, and the email job queue
//      (+ saved settings that override the environment)
//   4. smtp pool   - manages persistent, reusable SMTP connections
//   5. worker pool - runs background Go workers that claim jobs using FOR UPDATE SKIP LOCKED
//   6. http server - serves the REST API
//   7. shutdown    - gracefully stops workers and connections on SIGINT / SIGTERM
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sendmail/internal/api"
	"sendmail/internal/config"
	"sendmail/internal/domaincheck"
	"sendmail/internal/mailer"
	"sendmail/internal/migrations"
	"sendmail/internal/settings"
	"sendmail/internal/store"
	"sendmail/internal/worker"

	// Register the pgx PostgreSQL driver with database/sql
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	// 1. Load configuration from environment / .env
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// 2. Connect to PostgreSQL using standard library database/sql
	log.Printf("connecting to database...")
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database connection: %v", err)
	}
	defer db.Close()

	// Verify database connectivity
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if err := db.PingContext(pingCtx); err != nil {
		log.Printf("WARNING: failed to ping database (%v).", err)
		log.Printf("Please ensure PostgreSQL is running and DATABASE_URL in .env is correct.")
	} else {
		log.Printf("database connected successfully")
	}

	// 3. Bring the database schema up to date, then create the Store.
	//
	// Migrations are numbered SQL files embedded in the binary (see
	// internal/migrations). Failing here is fatal on purpose: running against a
	// missing or outdated schema would only fail later, on real requests.
	migCtx, migCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer migCancel()
	if cfg.AutoMigrate {
		applied, err := migrations.Up(migCtx, db)
		if err != nil {
			log.Fatalf("apply database migrations: %v", err)
		}
		if len(applied) == 0 {
			log.Printf("database schema is up to date")
		}
		for _, m := range applied {
			log.Printf("applied migration %s", m.Source.Path)
		}
	} else if pending, err := migrations.Pending(migCtx, db); err != nil {
		log.Fatalf("check database migrations: %v", err)
	} else if pending {
		log.Fatalf("database has pending migrations and AUTO_MIGRATE is off: run `go run ./cmd/migrate up` first")
	}
	st := store.New(db)

	// 3b. Settings edited from the dashboard override the environment. Load them
	// now so everything built below (workers, SMTP pool) starts with the
	// effective values. The Applier pushes later changes into the running app.
	encKey, err := settings.ParseKey(cfg.SettingsEncryptionKey)
	if err != nil {
		log.Fatalf("%v", err)
	}
	var (
		smtpPool *mailer.Pool
		handler  *api.Handler
	)
	settingsSvc := settings.New(st, cfg, encKey, func(c config.Config) {
		smtpPool.Reconfigure(c)
		st.SetMaxAttempts(c.MaxJobRetries)
		handler.SetDomainCheckEnabled(c.DomainCheckEnabled)
	})
	loadCtx, loadCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer loadCancel()
	if effective, err := settingsSvc.Load(loadCtx); err != nil {
		log.Printf("WARNING: could not load saved settings, using environment values: %v", err)
	} else {
		cfg = effective
	}
	st.SetMaxAttempts(cfg.MaxJobRetries)

	// Configure pool limits for database/sql
	db.SetMaxOpenConns(cfg.WorkerCount + 10)
	db.SetMaxIdleConns(cfg.WorkerCount + 2)
	db.SetConnMaxLifetime(30 * time.Minute)

	// 4. Initialize SMTP Connection Pool
	smtpPool = mailer.NewPool(cfg, cfg.SMTPPoolSize)
	defer smtpPool.Close()

	// 5. Initialize Background Worker Pool
	workerPool := worker.NewPool(cfg.WorkerCount, cfg.JobPollInterval, st, smtpPool)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	workerPool.Start(workerCtx)

	// 6. Build HTTP Handler & Router
	handler = api.NewHandler(smtpPool, st, workerPool)
	// The checker is always installed so the Settings page can switch it on and
	// off at runtime; DOMAIN_CHECK_ENABLED only decides the starting state.
	handler.WithDomainChecker(domaincheck.NewChecker(net.DefaultResolver))
	handler.SetDomainCheckEnabled(cfg.DomainCheckEnabled)
	handler.WithSettings(settingsSvc)
	if cfg.DomainCheckEnabled {
		log.Printf("recipient domain (DNS/MX) check enabled")
	}
	router := api.WithCORS(api.NewRouter(handler), cfg.CORSAllowedOrigins)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
	}

	// 7. Run HTTP Server in a separate goroutine so it doesn't block shutdown listening
	serverErrChan := make(chan error, 1)
	go func() {
		log.Printf("server listening on %s (workers: %d, smtp pool: %d)",
			cfg.HTTPAddr, cfg.WorkerCount, cfg.SMTPPoolSize)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrChan <- err
		}
	}()

	// 8. Listen for graceful shutdown signals (SIGINT, SIGTERM)
	shutdownSigChan := make(chan os.Signal, 1)
	signal.Notify(shutdownSigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrChan:
		log.Fatalf("http server error: %v", err)
	case sig := <-shutdownSigChan:
		log.Printf("received signal %v: initiating graceful shutdown...", sig)
	}

	// Graceful shutdown sequence:
	// a. Stop accepting new HTTP requests
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown error: %v", err)
	} else {
		log.Printf("http server stopped")
	}

	// b. Stop workers and wait for any active in-flight email sends to complete
	workerCancel()
	workerPool.Stop()

	// c. Close SMTP connections
	if err := smtpPool.Close(); err != nil {
		log.Printf("smtp pool close error: %v", err)
	}

	log.Printf("graceful shutdown complete. Bye!")
}

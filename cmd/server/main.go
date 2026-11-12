// Command server is the program's entry point.
//
// Convention: main packages live under cmd/<binary-name>/ and stay THIN.
// main's job is "wiring": build each piece, plug them together, start.
// All real logic lives in internal/ packages, where it can be tested.
//
// Systems wired together here:
//   1. config      - reads environment settings (.env), then sets up logging
//   2. database    - connects to PostgreSQL via database/sql (pgx driver)
//   3. store       - manages schema, batches, and the email job queue
//      (+ saved settings that override the environment)
//   4. smtp pool   - manages persistent, reusable SMTP connections
//   5. workers     - background goroutines in this same process (decision 0013):
//        import worker  reads uploaded spreadsheets into email_recipients
//        generator      creates email jobs after the user clicks Send
//        sending pool   claims jobs (FOR UPDATE SKIP LOCKED) and sends them
//   6. http server - serves the REST API
//   7. shutdown    - gracefully stops workers and connections on SIGINT / SIGTERM
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sendmail/internal/api"
	"sendmail/internal/config"
	"sendmail/internal/domaincheck"
	"sendmail/internal/importer"
	"sendmail/internal/logging"
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

	// 1b. Logging: levels, text/json, and optionally a copy in a file.
	// From here on every line (including old log.Printf calls) goes through it.
	logger, logFile, err := logging.Setup(logging.Options{
		Level: cfg.LogLevel, Format: cfg.LogFormat, File: cfg.LogFile,
	})
	if err != nil {
		log.Fatalf("set up logging: %v", err)
	}
	defer logFile.Close()
	slog.Info("logging ready", "log_level", cfg.LogLevel, "log_format", cfg.LogFormat, "log_file", cfg.LogFile)

	// 2. Connect to PostgreSQL using standard library database/sql
	slog.Info("connecting to database")
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		fatal("open database connection", err)
	}
	defer db.Close()

	// Verify database connectivity
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if err := db.PingContext(pingCtx); err != nil {
		slog.Warn("cannot reach the database; check that PostgreSQL is running and DATABASE_URL is correct", "err", err)
	} else {
		slog.Info("database connected")
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
			fatal("apply database migrations", err)
		}
		if len(applied) == 0 {
			slog.Info("database schema is up to date")
		}
		for _, m := range applied {
			slog.Info("applied migration", "file", m.Source.Path)
		}
	} else if pending, err := migrations.Pending(migCtx, db); err != nil {
		fatal("check database migrations", err)
	} else if pending {
		fatal("database has pending migrations and AUTO_MIGRATE is off: run `goose up` first (see Database migrations in the README)", nil)
	}
	st := store.New(db)

	// 3b. Settings edited from the dashboard override the environment. Load them
	// now so everything built below (workers, SMTP pool) starts with the
	// effective values. The Applier pushes later changes into the running app.
	encKey, err := settings.ParseKey(cfg.SettingsEncryptionKey)
	if err != nil {
		fatal("settings encryption key", err)
	}
	var (
		smtpPool     *mailer.Pool
		handler      *api.Handler
		importWorker *importer.Worker
		generator    *worker.Generator
	)
	settingsSvc := settings.New(st, cfg, encKey, func(c config.Config) {
		smtpPool.Reconfigure(c)
		st.SetMaxAttempts(c.MaxJobRetries)
		importWorker.SetDomainCheckEnabled(c.DomainCheckEnabled)
		importWorker.SetChunkSize(c.ImportChunkSize)
		generator.SetChunkSize(c.JobGenerationChunkSize)
		handler.SetMaxUploadMB(c.MaxUploadMB)
	})
	loadCtx, loadCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer loadCancel()
	if effective, err := settingsSvc.Load(loadCtx); err != nil {
		slog.Warn("could not load saved settings, using environment values", "err", err)
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

	// 5. Background workers. They all run in this process for now; they only
	// talk to each other through the database, so they could move to a separate
	// worker process later without changes (ROADMAP: "Separate worker process").
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	// 5a. Sending pool: claims email jobs and sends them.
	workerPool := worker.NewPool(cfg.WorkerCount, cfg.JobPollInterval, st, smtpPool)
	workerPool.Start(workerCtx)

	// 5b. Job generator: after Send, creates the email jobs, then wakes the senders.
	generator = worker.NewGenerator(st, time.Second, workerPool.Wake)
	generator.SetChunkSize(cfg.JobGenerationChunkSize)
	generator.Start(workerCtx)

	// 5c. Import worker: reads uploaded spreadsheets from UPLOAD_DIR. The
	// domain checker is always installed so the Settings page can switch it on
	// and off at runtime; DOMAIN_CHECK_ENABLED only decides the starting state.
	files := importer.Files{Dir: cfg.UploadDir}
	importWorker = importer.NewWorker(st, files, domaincheck.NewChecker(net.DefaultResolver), time.Second)
	importWorker.SetDomainCheckEnabled(cfg.DomainCheckEnabled)
	importWorker.SetChunkSize(cfg.ImportChunkSize)
	importWorker.Start(workerCtx)
	if cfg.DomainCheckEnabled {
		slog.Info("recipient domain (DNS/MX) check enabled")
	}

	// 5d. Every hour, remove upload files no batch will read (an upload that
	// died halfway, or a batch that was never created).
	go cleanupOrphanFiles(workerCtx, files, st, time.Hour)

	// 6. Build HTTP Handler & Router
	handler = api.NewHandler(smtpPool, st, workerPool).WithImports(files, importWorker, generator)
	handler.SetMaxUploadMB(cfg.MaxUploadMB)
	handler.WithSettings(settingsSvc)
	router := api.WithRequestLog(api.WithCORS(api.NewRouter(handler), cfg.CORSAllowedOrigins), logger)

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
		slog.Info("server listening", "addr", cfg.HTTPAddr, "workers", cfg.WorkerCount, "smtp_pool", cfg.SMTPPoolSize)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrChan <- err
		}
	}()

	// 8. Listen for graceful shutdown signals (SIGINT, SIGTERM)
	shutdownSigChan := make(chan os.Signal, 1)
	signal.Notify(shutdownSigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrChan:
		fatal("http server error", err)
	case sig := <-shutdownSigChan:
		slog.Info("shutting down", "signal", sig.String())
	}

	// Graceful shutdown sequence:
	// a. Stop accepting new HTTP requests
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown failed", "err", err)
	} else {
		slog.Info("http server stopped")
	}

	// b. Stop the workers. An import or job generation in progress keeps its
	// checkpoint and continues on the next start; emails being sent finish.
	workerCancel()
	importWorker.Stop()
	generator.Stop()
	workerPool.Stop()

	// c. Close SMTP connections
	if err := smtpPool.Close(); err != nil {
		slog.Error("smtp pool close failed", "err", err)
	}

	slog.Info("graceful shutdown complete")
}

// cleanupOrphanFiles removes orphan upload files every interval until ctx ends.
func cleanupOrphanFiles(ctx context.Context, files importer.Files, st *store.Store, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		n, err := files.CleanupOrphans(ctx, st, time.Hour)
		switch {
		case err != nil && ctx.Err() == nil:
			slog.Error("clean up orphan uploads failed", "err", err)
		case n > 0:
			slog.Info("orphan upload files removed", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// fatal logs an error and exits, like log.Fatalf but through the structured
// logger so the line also reaches the log file. Deferred cleanups do not run.
func fatal(msg string, err error) {
	if err != nil {
		slog.Error(msg, "err", err)
	} else {
		slog.Error(msg)
	}
	os.Exit(1)
}

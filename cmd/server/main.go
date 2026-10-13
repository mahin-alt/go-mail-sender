// Command server is the program's entry point.
//
// Convention: main packages live under cmd/<binary-name>/ and stay THIN.
// main's only job is "wiring": build each piece, plug them together, start.
// All real logic lives in internal/ packages, where it can be tested.
package main

import (
	"log"
	"net/http"
	"time"

	"sendmail/internal/api"
	"sendmail/internal/config"
	"sendmail/internal/mailer"
)

func main() {
	// Wiring, from the inside out:
	//   config -> mailer -> handler -> router -> http server
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	smtpMailer := mailer.NewSMTPMailer(cfg)
	handler := api.NewHandler(smtpMailer)
	router := api.NewRouter(handler)

	// Always build your own http.Server instead of http.ListenAndServe:
	// the default has NO timeouts, so a slow or malicious client could hold
	// connections open forever.
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		// Sending mail talks to a remote SMTP server, so leave room for it.
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("listening on %s", cfg.HTTPAddr)
	// ListenAndServe blocks until the server stops; it only ever returns an error.
	log.Fatal(srv.ListenAndServe())
}

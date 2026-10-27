package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// TLS modes for the connection to the SMTP server.
//
// Concept: SMTP started out unencrypted. Today there are two ways to add
// encryption: "STARTTLS" (connect in plain text, then upgrade in place; the
// usual choice on port 587) and "implicit TLS" (encrypted from the very first
// byte; the usual choice on port 465).
const (
	TLSAuto     = "auto"     // STARTTLS if the server offers it, otherwise plain
	TLSNone     = "none"     // never encrypt
	TLSStartTLS = "starttls" // require STARTTLS
	TLSImplicit = "tls"      // implicit TLS
)

// ConnConfig is everything needed to open one authenticated SMTP connection.
// It is separate from config.Config so the Settings "test" feature can try
// values that have not been saved yet.
type ConnConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	TLSMode  string // one of the TLS* constants; "" means TLSAuto

	// tlsConfig overrides the default TLS settings. Tests use it to trust a
	// self-signed certificate; production code leaves it nil.
	tlsConfig *tls.Config
}

// dialTimeout bounds connecting and negotiating when the caller's context has
// no deadline of its own.
const dialTimeout = 30 * time.Second

// Dial opens a connection to the SMTP server, negotiates encryption according
// to c.TLSMode, and authenticates if credentials are set. The caller owns the
// returned client and must Quit or Close it.
func Dial(ctx context.Context, c ConnConfig) (*smtp.Client, error) {
	mode := c.TLSMode
	if mode == "" {
		mode = TLSAuto
	}

	addr := net.JoinHostPort(c.Host, c.Port)
	tlsCfg := c.tlsConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	}

	// Every step below (connect, TLS handshake, greeting, STARTTLS, AUTH) is
	// bounded by one deadline so a silent server cannot hang us forever.
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(dialTimeout)
	}

	var (
		conn net.Conn
		err  error
	)
	if mode == TLSImplicit {
		d := tls.Dialer{NetDialer: &net.Dialer{Deadline: deadline}, Config: tlsCfg}
		conn, err = d.DialContext(ctx, "tcp", addr)
	} else {
		d := net.Dialer{Deadline: deadline}
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", addr, err)
	}
	_ = conn.SetDeadline(deadline)

	client, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("smtp greeting from %s: %w", addr, err)
	}

	// Upgrade to TLS in place. (Not needed after implicit TLS, which is
	// already encrypted.) client.StartTLS re-sends EHLO afterwards, so the
	// extension list we read next (notably AUTH) is the post-TLS one.
	if mode == TLSAuto || mode == TLSStartTLS {
		offered, _ := client.Extension("STARTTLS")
		switch {
		case offered:
			if err := client.StartTLS(tlsCfg); err != nil {
				_ = client.Close()
				return nil, fmt.Errorf("starttls with %s: %w", addr, err)
			}
		case mode == TLSStartTLS:
			_ = client.Close()
			return nil, fmt.Errorf("%s does not offer STARTTLS (TLS mode is %q)", addr, TLSStartTLS)
		}
	}

	// Authenticate ONLY if credentials are provided, and only if the server
	// advertises AUTH (Mailpit by default does not).
	if c.Username != "" && c.Password != "" {
		if ok, _ := client.Extension("AUTH"); ok {
			auth := smtp.PlainAuth("", c.Username, c.Password, c.Host)
			if err := client.Auth(auth); err != nil {
				_ = client.Close()
				msg := err.Error()
				if strings.Contains(msg, "unencrypted connection") {
					msg += " (the password would be sent unencrypted: set the TLS mode to starttls or tls)"
				}
				return nil, fmt.Errorf("smtp auth: %s", msg)
			}
		}
	}

	_ = conn.SetDeadline(time.Time{}) // later commands use the caller's own timeouts
	return client, nil
}

// Check opens a connection (including TLS and login) and closes it again,
// without sending anything. It reports whether the settings are usable.
func Check(ctx context.Context, c ConnConfig) error {
	client, err := Dial(ctx, c)
	if err != nil {
		return err
	}
	_ = client.Quit()
	return nil
}

// SendTest sends one small email over a brand-new connection built from c.
// It never touches the running connection pool.
func SendTest(ctx context.Context, c ConnConfig, from, to string) error {
	client, err := Dial(ctx, c)
	if err != nil {
		return err
	}
	defer client.Close()

	body := "This is a test email from the bulk email dashboard.\n\n" +
		"If you can read this, your SMTP settings work."
	if err := deliver(client, from, to, "SMTP test email", body); err != nil {
		return err
	}
	_ = client.Quit()
	return nil
}

// deliver runs one SMTP transaction (MAIL FROM, RCPT TO, DATA) on an open client.
func deliver(client *smtp.Client, from, to, subject, body string) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt to %q: %w", to, err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data command: %w", err)
	}
	if _, err := w.Write(buildMessage(from, to, subject, body)); err != nil {
		_ = w.Close()
		return fmt.Errorf("write message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close message data: %w", err)
	}
	return nil
}

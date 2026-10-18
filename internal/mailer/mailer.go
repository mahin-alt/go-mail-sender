// Package mailer knows how to deliver an email. It knows NOTHING about HTTP.
//
// Concept: separation of concerns. The HTTP layer decides "should we send
// this?", this package decides "how do we send it?". Because they are
// separate, you could swap SMTP for an email API (SendGrid, Mailtrap's API...)
// without touching the HTTP code.
package mailer

import (
	"net/smtp"

	"sendmail/internal/config"
)

// Mailer is the behaviour the rest of the app depends on.
//
// Concept: interfaces in Go are satisfied implicitly - any type with a
// matching Send method IS a Mailer, no "implements" keyword. Idiomatic Go
// says: define the interface where it is USED. We define it here only to
// keep this tiny project readable; the API package accepts it as a
// dependency, which lets tests pass in a fake instead of really sending mail.
type Mailer interface {
	Send(to, subject, body string) error
}

// SMTPMailer is the single-shot implementation, speaking SMTP via the standard library.
// For higher throughput and connection reuse, see Pool in pool.go.
type SMTPMailer struct {
	cfg config.Config
}

// NewSMTPMailer is a constructor. Go has no constructors built in; by
// convention a function named NewXxx returns a ready-to-use value.
func NewSMTPMailer(cfg config.Config) *SMTPMailer {
	return &SMTPMailer{cfg: cfg}
}

// Send delivers a plain-text email from the configured sender to `to`.
func (m *SMTPMailer) Send(to, subject, body string) error {
	var auth smtp.Auth
	// Only supply authentication if credentials are provided (Mailtrap/prod).
	// When using Mailpit on localhost without credentials, leave auth as nil.
	if m.cfg.SMTPUsername != "" && m.cfg.SMTPPassword != "" {
		auth = smtp.PlainAuth("", m.cfg.SMTPUsername, m.cfg.SMTPPassword, m.cfg.SMTPHost)
	}

	// An email is just text: headers, a blank line, then the body.
	// SMTP requires "\r\n" (CRLF) line endings.
	// NOTE: `subject` and `to` are put straight into headers, so they must
	// not contain "\r\n" - otherwise a caller could inject extra headers
	// (e.g. a hidden Bcc). The API layer validates this before calling us.
	message := buildMessage(m.cfg.MailFrom, to, subject, body)

	// SendMail connects, authenticates (upgrading to TLS via STARTTLS when
	// the server offers it), sends the message, and disconnects.
	// Note the "envelope" (from/to arguments) is separate from the headers
	// inside the message: the envelope is who the server actually delivers to.
	return smtp.SendMail(
		m.cfg.SMTPHost+":"+m.cfg.SMTPPort,
		auth,
		m.cfg.MailFrom,
		[]string{to},
		message,
	)
}

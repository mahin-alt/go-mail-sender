package settings

import (
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"sendmail/internal/config"
)

// How a setting takes effect after it is saved.
const (
	ApplyInstant  = "instant"  // takes effect immediately, no restart
	ApplyRestart  = "restart"  // saved now, used the next time the server starts
	ApplyReadOnly = "readonly" // shown for reference, never editable here
)

// def describes one setting: how to read it from a Config, validate and
// normalise a new value, and write it back into a Config.
type def struct {
	key     string
	desc    string
	kind    string // string | int | bool | duration | enum
	secret  bool
	apply   string
	options []string

	// parse validates raw input and returns the canonical stored form.
	// nil for read-only settings.
	parse func(raw string) (string, error)
	get   func(c config.Config) string
	set   func(c *config.Config, v string)
}

func (d def) editable() bool { return d.apply != ApplyReadOnly }

// catalog lists every setting shown on the Settings page, in display order.
// DATABASE_URL is deliberately absent: it holds credentials and a wrong value
// would lock the app out of its own database, so it stays in the environment.
var catalog = []def{
	{
		key: "MAIL_FROM", desc: "Sender address on every outgoing email", kind: "string", apply: ApplyInstant,
		parse: parseSender,
		get:   func(c config.Config) string { return c.MailFrom },
		set:   func(c *config.Config, v string) { c.MailFrom = v },
	},
	{
		key: "SMTP_HOST", desc: "SMTP server hostname", kind: "string", apply: ApplyInstant,
		parse: parseHost,
		get:   func(c config.Config) string { return c.SMTPHost },
		set:   func(c *config.Config, v string) { c.SMTPHost = v },
	},
	{
		key: "SMTP_PORT", desc: "SMTP server port (587 for STARTTLS, 465 for implicit TLS, 1025 for Mailpit)", kind: "int", apply: ApplyInstant,
		parse: intRange("SMTP_PORT", 1, 65535),
		get:   func(c config.Config) string { return c.SMTPPort },
		set:   func(c *config.Config, v string) { c.SMTPPort = v },
	},
	{
		key: "SMTP_TLS_MODE", desc: "Encryption to the SMTP server", kind: "enum", apply: ApplyInstant,
		options: []string{"auto", "none", "starttls", "tls"},
		parse:   parseTLSMode,
		get:     func(c config.Config) string { return c.SMTPTLSMode },
		set:     func(c *config.Config, v string) { c.SMTPTLSMode = v },
	},
	{
		key: "SMTP_USERNAME", desc: "SMTP login (leave empty for servers without authentication)", kind: "string", apply: ApplyInstant,
		parse: parseCredential("SMTP_USERNAME", 255),
		get:   func(c config.Config) string { return c.SMTPUsername },
		set:   func(c *config.Config, v string) { c.SMTPUsername = v },
	},
	{
		key: "SMTP_PASSWORD", desc: "SMTP password (write-only: never shown again after saving)", kind: "string", secret: true, apply: ApplyInstant,
		parse: parseCredential("SMTP_PASSWORD", 1024),
		get:   func(c config.Config) string { return c.SMTPPassword },
		set:   func(c *config.Config, v string) { c.SMTPPassword = v },
	},
	{
		key: "DOMAIN_CHECK_ENABLED", desc: "Reject recipients whose domain has no mail server (DNS/MX check)", kind: "bool", apply: ApplyInstant,
		parse: parseBool("DOMAIN_CHECK_ENABLED"),
		get:   func(c config.Config) string { return strconv.FormatBool(c.DomainCheckEnabled) },
		set:   func(c *config.Config, v string) { c.DomainCheckEnabled, _ = strconv.ParseBool(v) },
	},
	{
		key: "MAX_JOB_RETRIES", desc: "Delivery attempts per email before it is marked failed (applies to new uploads)", kind: "int", apply: ApplyInstant,
		parse: intRange("MAX_JOB_RETRIES", 1, 20),
		get:   func(c config.Config) string { return strconv.Itoa(c.MaxJobRetries) },
		set:   func(c *config.Config, v string) { c.MaxJobRetries, _ = strconv.Atoi(v) },
	},
	{
		key: "WORKER_COUNT", desc: "Background workers sending emails in parallel", kind: "int", apply: ApplyRestart,
		parse: intRange("WORKER_COUNT", 1, 100),
		get:   func(c config.Config) string { return strconv.Itoa(c.WorkerCount) },
		set:   func(c *config.Config, v string) { c.WorkerCount, _ = strconv.Atoi(v) },
	},
	{
		key: "SMTP_POOL_SIZE", desc: "Maximum simultaneous SMTP connections", kind: "int", apply: ApplyRestart,
		parse: intRange("SMTP_POOL_SIZE", 1, 100),
		get:   func(c config.Config) string { return strconv.Itoa(c.SMTPPoolSize) },
		set:   func(c *config.Config, v string) { c.SMTPPoolSize, _ = strconv.Atoi(v) },
	},
	{
		key: "JOB_POLL_INTERVAL", desc: "How often idle workers check for new jobs (e.g. 500ms)", kind: "duration", apply: ApplyRestart,
		parse: parseDuration("JOB_POLL_INTERVAL", 50*time.Millisecond, time.Minute),
		get:   func(c config.Config) string { return c.JobPollInterval.String() },
		set:   func(c *config.Config, v string) { c.JobPollInterval, _ = time.ParseDuration(v) },
	},
	{
		key: "HTTP_ADDR", desc: "Address the API listens on (changing it would cut off this page)", kind: "string", apply: ApplyReadOnly,
		get: func(c config.Config) string { return c.HTTPAddr },
	},
	{
		key: "HTTP_READ_TIMEOUT", desc: "Maximum time to read a request, including uploads", kind: "duration", apply: ApplyReadOnly,
		get: func(c config.Config) string { return c.HTTPReadTimeout.String() },
	},
	{
		key: "HTTP_WRITE_TIMEOUT", desc: "Maximum time a request may take before its response is written", kind: "duration", apply: ApplyReadOnly,
		get: func(c config.Config) string { return c.HTTPWriteTimeout.String() },
	},
}

func findDef(key string) (def, bool) {
	for _, d := range catalog {
		if d.key == key {
			return d, true
		}
	}
	return def{}, false
}

// smtpKeys are the settings that change how we connect to the mail server.
// Changing any of them triggers a live connection check before saving.
var smtpKeys = map[string]bool{
	"SMTP_HOST": true, "SMTP_PORT": true, "SMTP_TLS_MODE": true,
	"SMTP_USERNAME": true, "SMTP_PASSWORD": true,
}

func hasCRLF(s string) bool { return strings.ContainsAny(s, "\r\n\x00") }

func parseSender(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	addr, err := mail.ParseAddress(v)
	if err != nil || addr.Address != v || hasCRLF(v) {
		return "", errors.New("MAIL_FROM must be a plain email address like hello@example.com")
	}
	return v, nil
}

func parseHost(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" || len(v) > 253 || strings.ContainsAny(v, " \t/\\@") || hasCRLF(v) {
		return "", errors.New("SMTP_HOST must be a hostname or IP address, without spaces or slashes")
	}
	return v, nil
}

func parseTLSMode(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if !config.ValidTLSMode(v) {
		return "", errors.New("SMTP_TLS_MODE must be one of: auto, none, starttls, tls")
	}
	return v, nil
}

func parseCredential(name string, max int) func(string) (string, error) {
	return func(raw string) (string, error) {
		if hasCRLF(raw) || len(raw) > max {
			return "", fmt.Errorf("%s must be at most %d characters and contain no line breaks", name, max)
		}
		return raw, nil // not trimmed: passwords may legitimately contain spaces
	}
}

func intRange(name string, lo, hi int) func(string) (string, error) {
	return func(raw string) (string, error) {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < lo || n > hi {
			return "", fmt.Errorf("%s must be a whole number between %d and %d", name, lo, hi)
		}
		return strconv.Itoa(n), nil
	}
}

func parseBool(name string) func(string) (string, error) {
	return func(raw string) (string, error) {
		b, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return "", fmt.Errorf("%s must be true or false", name)
		}
		return strconv.FormatBool(b), nil
	}
}

func parseDuration(name string, lo, hi time.Duration) func(string) (string, error) {
	return func(raw string) (string, error) {
		d, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil || d < lo || d > hi {
			return "", fmt.Errorf("%s must be a duration between %s and %s (e.g. 500ms)", name, lo, hi)
		}
		return d.String(), nil
	}
}

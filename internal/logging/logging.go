// Package logging sets up the program's logger.
//
// Concept: structured logging with levels. Every line has a level (DEBUG <
// INFO < WARN < ERROR) and key=value fields instead of free text, e.g.
//
//	level=WARN msg="send failed, will retry" job_id=92 attempt=1
//
// LOG_LEVEL is a minimum: lines below it are not written at all. We use the
// standard library's log/slog, so there is no extra dependency.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Options say what to write and where.
type Options struct {
	Level  string // debug | info | warn | error
	Format string // text | json
	File   string // optional copy of every line, e.g. "logs/server.log"; "" = terminal only
}

// Setup builds the logger and installs it as the default, so slog.Info(...)
// anywhere in the program uses it. It also captures the standard library's
// log package: any remaining log.Printf lines come out as INFO.
//
// The returned Closer closes the log file (a no-op without one); call it on
// shutdown. A file that cannot be opened is an error: better to stop at
// startup than to run without the logs you expect.
func Setup(o Options) (*slog.Logger, io.Closer, error) {
	level, err := ParseLevel(o.Level)
	if err != nil {
		return nil, nil, err
	}

	var out io.Writer = os.Stderr
	var closer io.Closer = nopCloser{}
	if o.File != "" {
		if err := os.MkdirAll(filepath.Dir(o.File), 0o755); err != nil {
			return nil, nil, fmt.Errorf("create log folder: %w", err)
		}
		f, err := os.OpenFile(o.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file: %w", err)
		}
		// Every line goes to the terminal AND the file.
		out = io.MultiWriter(os.Stderr, f)
		closer = f
	}

	logger := New(out, level, o.Format)
	slog.SetDefault(logger)
	return logger, closer, nil
}

// New builds a logger writing to w. Tests use it with a buffer.
func New(w io.Writer, level slog.Level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	if format == "json" {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}

// ParseLevel turns "debug", "info", "warn" or "error" into a slog.Level.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("log level %q is invalid: use debug, info, warn or error", s)
}

// MaskEmail hides most of the mailbox name so logs do not hold customers'
// addresses, while keeping the domain (useful to spot a provider outage):
// "aisha.rahman@gmail.com" -> "a***@gmail.com".
func MaskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}
	return email[:1] + "***" + email[at:]
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

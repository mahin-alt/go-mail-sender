// Package settings lets selected environment settings be changed at runtime
// from the dashboard.
//
// Concept: layered configuration. The environment (.env) supplies the base
// values; the app_settings table holds overrides edited in the UI; the
// effective value is the override if present, otherwise the environment's.
// "Reset" simply deletes the override. Some settings apply instantly (the SMTP
// pool is rebuilt live); others are only read at startup and are flagged
// "restart required".
package settings

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"sort"
	"strings"
	"sync"
	"time"

	"sendmail/internal/config"
	"sendmail/internal/mailer"
	"sendmail/internal/store"
)

// InputError marks problems caused by what the user submitted (bad value,
// unknown key, SMTP settings that do not work). The API turns it into a 400.
type InputError struct{ Msg string }

func (e *InputError) Error() string { return e.Msg }

func inputErr(format string, args ...any) error {
	return &InputError{Msg: fmt.Sprintf(format, args...)}
}

// smtpCheckTimeout bounds the live connection check and the test-send.
const smtpCheckTimeout = 20 * time.Second

// Applier is called with the new effective config after a successful save, so
// the running app can adopt instant settings (rebuild SMTP pool, etc.).
type Applier func(cfg config.Config)

// Service reads, validates, stores and applies settings.
type Service struct {
	store *store.Store
	env   config.Config // base values from the environment, never modified
	key   []byte        // encryption key for secrets; nil = secrets cannot be stored
	apply Applier

	// checkSMTP verifies SMTP settings by really connecting. A field so tests
	// can substitute a fake.
	checkSMTP func(ctx context.Context, c mailer.ConnConfig) error

	mu        sync.RWMutex
	current   config.Config // effective now: env + overrides
	running   config.Config // what the process started with
	overrides map[string]store.SettingRow
}

// New creates the service. apply may be nil.
func New(st *store.Store, env config.Config, key []byte, apply Applier) *Service {
	return &Service{
		store: st, env: env, key: key, apply: apply,
		checkSMTP: mailer.Check,
		current:   env, running: env,
		overrides: map[string]store.SettingRow{},
	}
}

// Load reads the stored overrides and returns the effective config. Call it
// once at startup, before building anything that depends on the settings.
// An override that cannot be decrypted or no longer validates is skipped with a
// log line, so a bad row never stops the server from starting.
func (s *Service) Load(ctx context.Context) (config.Config, error) {
	rows, err := s.store.GetSettings(ctx)
	if err != nil {
		return s.env, err
	}

	cfg := s.env
	kept := map[string]store.SettingRow{}
	for key, row := range rows {
		d, ok := findDef(key)
		if !ok || !d.editable() {
			continue
		}
		val := row.Value
		if row.Secret {
			if val, err = decrypt(s.key, row.Value); err != nil {
				log.Printf("settings: ignoring stored %s: %v", key, err)
				continue
			}
		}
		if val, err = d.parse(val); err != nil {
			log.Printf("settings: ignoring stored %s: %v", key, err)
			continue
		}
		d.set(&cfg, val)
		kept[key] = row
	}

	s.mu.Lock()
	s.current, s.running, s.overrides = cfg, cfg, kept
	s.mu.Unlock()
	return cfg, nil
}

// Effective returns the current effective config.
func (s *Service) Effective() config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// Item is one row of the Settings page.
type Item struct {
	Key             string     `json:"key"`
	Description     string     `json:"description"`
	Kind            string     `json:"kind"`
	Value           string     `json:"value"`     // always empty for secrets
	HasValue        bool       `json:"has_value"` // for secrets: is one configured?
	Source          string     `json:"source"`    // "env" or "override"
	Editable        bool       `json:"editable"`
	Secret          bool       `json:"secret"`
	Apply           string     `json:"apply"`
	Options         []string   `json:"options,omitempty"`
	RestartRequired bool       `json:"restart_required"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// List returns every setting with its effective value and where it came from.
func (s *Service) List() []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listLocked()
}

func (s *Service) listLocked() []Item {
	items := make([]Item, 0, len(catalog))
	for _, d := range catalog {
		cur := d.get(s.current)
		it := Item{
			Key: d.key, Description: d.desc, Kind: d.kind, Source: "env",
			Editable: d.editable(), Secret: d.secret, Apply: d.apply, Options: d.options,
			HasValue:        cur != "",
			RestartRequired: d.apply == ApplyRestart && cur != d.get(s.running),
		}
		if !d.secret {
			it.Value = cur
		}
		if row, ok := s.overrides[d.key]; ok {
			it.Source = "override"
			t := row.UpdatedAt
			it.UpdatedAt = &t
		}
		items = append(items, it)
	}
	return items
}

// Change is a settings edit: new values plus keys to reset to their environment default.
type Change struct {
	Values map[string]string `json:"values"`
	Reset  []string          `json:"reset"`
}

// Update validates and saves a change, applies instant settings, and returns the
// refreshed list. It is all-or-nothing: any invalid value rejects the whole change.
//
// If an SMTP setting changes, the new values are first tried against the real
// server (connect, encrypt, log in) and the change is refused if that fails,
// so a typo can never silently break all sending.
func (s *Service) Update(ctx context.Context, ch Change) ([]Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cand := s.current
	set := map[string]store.SettingRow{}
	var remove []string
	var changed []string

	for _, key := range ch.Reset {
		d, ok := findDef(key)
		if !ok || !d.editable() {
			return nil, inputErr("%s cannot be reset: unknown or read-only setting", key)
		}
		if _, hasOverride := s.overrides[key]; hasOverride {
			remove = append(remove, key)
		}
		if _, alsoSet := ch.Values[key]; alsoSet {
			return nil, inputErr("%s appears in both values and reset", key)
		}
		d.set(&cand, d.get(s.env))
		changed = append(changed, key)
	}

	for key, raw := range ch.Values {
		d, ok := findDef(key)
		if !ok {
			return nil, inputErr("unknown setting %q", key)
		}
		if !d.editable() {
			return nil, inputErr("%s is read-only; change it in the server environment", key)
		}
		val, err := d.parse(raw)
		if err != nil {
			return nil, &InputError{Msg: err.Error()}
		}
		if val == d.get(s.current) {
			continue // no change; do not create a pointless override
		}
		stored := val
		if d.secret {
			if stored, err = encrypt(s.key, val); err != nil {
				if errors.Is(err, ErrNoEncryptionKey) {
					return nil, inputErr("%s cannot be saved: SETTINGS_ENCRYPTION_KEY is not configured on the server", key)
				}
				return nil, fmt.Errorf("encrypt %s: %w", key, err)
			}
		}
		d.set(&cand, val)
		set[key] = store.SettingRow{Value: stored, Secret: d.secret}
		changed = append(changed, key)
	}

	if len(set) == 0 && len(remove) == 0 {
		return s.listLocked(), nil
	}

	if smtpChanged(s.current, cand) {
		ctx, cancel := context.WithTimeout(ctx, smtpCheckTimeout)
		defer cancel()
		if err := s.checkSMTP(ctx, connConfig(cand)); err != nil {
			return nil, inputErr("not saved: the new SMTP settings do not work: %v", err)
		}
	}

	if err := s.store.SaveSettings(ctx, set, remove); err != nil {
		return nil, err
	}

	now := time.Now()
	for key, row := range set {
		row.UpdatedAt = now
		s.overrides[key] = row
	}
	for _, key := range remove {
		delete(s.overrides, key)
	}
	s.current = cand
	if s.apply != nil {
		s.apply(cand)
	}

	sort.Strings(changed)
	log.Printf("settings updated: %s", strings.Join(changed, ", ")) // names only, never values
	return s.listLocked(), nil
}

func smtpChanged(a, b config.Config) bool {
	for key := range smtpKeys {
		d, _ := findDef(key)
		if d.get(a) != d.get(b) {
			return true
		}
	}
	return false
}

func connConfig(c config.Config) mailer.ConnConfig {
	return mailer.ConnConfig{
		Host: c.SMTPHost, Port: c.SMTPPort,
		Username: c.SMTPUsername, Password: c.SMTPPassword, TLSMode: c.SMTPTLSMode,
	}
}

// TestRequest asks for a real test email. Any SMTP field left nil falls back
// to the current effective value, so the stored password can be tested without
// retyping it, and unsaved edits can be tried before saving.
type TestRequest struct {
	To       string  `json:"to"`
	Host     *string `json:"host"`
	Port     *string `json:"port"`
	TLSMode  *string `json:"tls_mode"`
	Username *string `json:"username"`
	Password *string `json:"password"`
	From     *string `json:"from"`
}

// SendTest sends one test email using a fresh connection built from the
// request. It never touches the live SMTP pool and saves nothing. Problems with
// the request return *InputError; failures talking to the SMTP server return a
// plain error carrying the server's reason.
func (s *Service) SendTest(ctx context.Context, req TestRequest) (to string, err error) {
	to = strings.TrimSpace(req.To)
	if addr, perr := mail.ParseAddress(to); perr != nil || addr.Address != to || hasCRLF(to) {
		return "", inputErr(`"to" must be a plain email address`)
	}

	cfg := s.Effective()
	fields := []struct {
		key string
		val *string
	}{
		{"SMTP_HOST", req.Host}, {"SMTP_PORT", req.Port}, {"SMTP_TLS_MODE", req.TLSMode},
		{"SMTP_USERNAME", req.Username}, {"SMTP_PASSWORD", req.Password}, {"MAIL_FROM", req.From},
	}
	for _, f := range fields {
		if f.val == nil {
			continue
		}
		d, _ := findDef(f.key)
		v, perr := d.parse(*f.val)
		if perr != nil {
			return "", &InputError{Msg: perr.Error()}
		}
		d.set(&cfg, v)
	}

	ctx, cancel := context.WithTimeout(ctx, smtpCheckTimeout)
	defer cancel()
	if err := mailer.SendTest(ctx, connConfig(cfg), cfg.MailFrom, to); err != nil {
		return to, err
	}
	return to, nil
}

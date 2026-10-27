-- +goose Up
-- Settings edited from the dashboard. A row overrides the value of the
-- same-named environment variable. Secret values (e.g. SMTP_PASSWORD) are
-- stored encrypted (AES-256-GCM, key from SETTINGS_ENCRYPTION_KEY).
CREATE TABLE IF NOT EXISTS app_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    secret BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS app_settings;

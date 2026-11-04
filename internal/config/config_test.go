package config

import "testing"

func TestLoad_MaxUploadMB(t *testing.T) {
	cases := []struct {
		env     string
		want    int
		wantErr bool
	}{
		{"", DefaultMaxUploadMB, false},
		{"40", 40, false},
		{"50", 50, false},
		{"0", DefaultMaxUploadMB, false},    // not positive: falls back like other ints
		{"lots", DefaultMaxUploadMB, false}, // not a number: falls back
		{"51", 0, true},
	}
	for _, c := range cases {
		t.Setenv("MAX_UPLOAD_MB", c.env)
		cfg, err := Load()
		if (err != nil) != c.wantErr {
			t.Fatalf("MAX_UPLOAD_MB=%q: err = %v, wantErr %v", c.env, err, c.wantErr)
		}
		if err == nil && cfg.MaxUploadMB != c.want {
			t.Errorf("MAX_UPLOAD_MB=%q: got %d, want %d", c.env, cfg.MaxUploadMB, c.want)
		}
	}
}

func TestLoad_Logging(t *testing.T) {
	t.Setenv("LOG_LEVEL", "WARN")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("LOG_FILE", " logs/server.log ")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != "warn" || cfg.LogFormat != "json" || cfg.LogFile != "logs/server.log" {
		t.Errorf("got level=%q format=%q file=%q", cfg.LogLevel, cfg.LogFormat, cfg.LogFile)
	}

	t.Setenv("LOG_LEVEL", "verbose")
	if _, err := Load(); err == nil {
		t.Error("unknown LOG_LEVEL should fail")
	}
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "xml")
	if _, err := Load(); err == nil {
		t.Error("unknown LOG_FORMAT should fail")
	}
}

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

func TestLoad_ChunkSizes(t *testing.T) {
	cases := []struct {
		key     string
		env     string
		want    int
		wantErr bool
		pick    func(Config) int
	}{
		{"IMPORT_CHUNK_SIZE", "", DefaultImportChunkSize, false, func(c Config) int { return c.ImportChunkSize }},
		{"IMPORT_CHUNK_SIZE", "2500", 2500, false, func(c Config) int { return c.ImportChunkSize }},
		{"IMPORT_CHUNK_SIZE", "50000", 50000, false, func(c Config) int { return c.ImportChunkSize }},
		{"IMPORT_CHUNK_SIZE", "0", DefaultImportChunkSize, false, func(c Config) int { return c.ImportChunkSize }},
		{"IMPORT_CHUNK_SIZE", "abc", DefaultImportChunkSize, false, func(c Config) int { return c.ImportChunkSize }},
		{"IMPORT_CHUNK_SIZE", "50001", 0, true, func(c Config) int { return c.ImportChunkSize }},
		{"JOB_GENERATION_CHUNK_SIZE", "", DefaultJobGenerationChunkSize, false, func(c Config) int { return c.JobGenerationChunkSize }},
		{"JOB_GENERATION_CHUNK_SIZE", "10000", 10000, false, func(c Config) int { return c.JobGenerationChunkSize }},
		{"JOB_GENERATION_CHUNK_SIZE", "100000", 100000, false, func(c Config) int { return c.JobGenerationChunkSize }},
		{"JOB_GENERATION_CHUNK_SIZE", "0", DefaultJobGenerationChunkSize, false, func(c Config) int { return c.JobGenerationChunkSize }},
		{"JOB_GENERATION_CHUNK_SIZE", "lots", DefaultJobGenerationChunkSize, false, func(c Config) int { return c.JobGenerationChunkSize }},
		{"JOB_GENERATION_CHUNK_SIZE", "100001", 0, true, func(c Config) int { return c.JobGenerationChunkSize }},
	}
	for _, c := range cases {
		t.Run(c.key+"="+c.env, func(t *testing.T) {
			t.Setenv("IMPORT_CHUNK_SIZE", "")
			t.Setenv("JOB_GENERATION_CHUNK_SIZE", "")
			t.Setenv(c.key, c.env)
			cfg, err := Load()
			if (err != nil) != c.wantErr {
				t.Fatalf("%s=%q: err = %v, wantErr %v", c.key, c.env, err, c.wantErr)
			}
			if err == nil && c.pick(cfg) != c.want {
				t.Errorf("%s=%q: got %d, want %d", c.key, c.env, c.pick(cfg), c.want)
			}
		})
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

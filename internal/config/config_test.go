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

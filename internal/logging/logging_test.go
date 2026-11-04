package logging

import (
	"bytes"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaskEmail(t *testing.T) {
	cases := map[string]string{
		"aisha.rahman@gmail.com": "a***@gmail.com",
		"b@yahoo.com":            "b***@yahoo.com",
		"no-at-sign":             "***",
		"@nolocal.com":           "***",
		"":                       "***",
	}
	for in, want := range cases {
		if got := MaskEmail(in); got != want {
			t.Errorf("MaskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]slog.Level{
		"debug": slog.LevelDebug, "INFO": slog.LevelInfo, "": slog.LevelInfo,
		" warn ": slog.LevelWarn, "error": slog.LevelError,
	} {
		got, err := ParseLevel(in)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseLevel("verbose"); err == nil {
		t.Error("an unknown level should be an error")
	}
}

func TestNew_LevelIsAMinimum(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, slog.LevelWarn, "text")
	l.Debug("d")
	l.Info("i")
	l.Warn("w")
	l.Error("e")
	out := buf.String()
	if strings.Contains(out, "msg=d") || strings.Contains(out, "msg=i") {
		t.Errorf("below-minimum lines were written:\n%s", out)
	}
	if !strings.Contains(out, "level=WARN msg=w") || !strings.Contains(out, "level=ERROR msg=e") {
		t.Errorf("WARN and ERROR lines missing:\n%s", out)
	}
}

func TestNew_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, slog.LevelInfo, "json").Info("batch completed", "batch_id", 3)
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("not JSON: %v (%s)", err, buf.String())
	}
	if line["msg"] != "batch completed" || line["batch_id"] != float64(3) || line["level"] != "INFO" {
		t.Errorf("unexpected line: %v", line)
	}
}

func TestSetup_WritesToFileAndCapturesStdLog(t *testing.T) {
	prevDefault, prevFlags, prevWriter := slog.Default(), log.Flags(), log.Writer()
	t.Cleanup(func() {
		slog.SetDefault(prevDefault)
		log.SetFlags(prevFlags)
		log.SetOutput(prevWriter)
	})

	file := filepath.Join(t.TempDir(), "logs", "server.log") // folder does not exist yet
	_, closer, err := Setup(Options{Level: "info", Format: "text", File: file})
	if err != nil {
		t.Fatal(err)
	}
	slog.Debug("hidden")
	slog.Warn("disk almost full", "pct", 91)
	log.Printf("old style line")
	closer.Close()

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if strings.Contains(out, "hidden") {
		t.Errorf("DEBUG line written at info level:\n%s", out)
	}
	if !strings.Contains(out, `level=WARN msg="disk almost full" pct=91`) {
		t.Errorf("WARN line missing:\n%s", out)
	}
	if !strings.Contains(out, `level=INFO msg="old style line"`) {
		t.Errorf("log.Printf should come out as INFO:\n%s", out)
	}
}

func TestSetup_BadInput(t *testing.T) {
	if _, _, err := Setup(Options{Level: "loud"}); err == nil {
		t.Error("bad level should fail")
	}
	// A path under a regular file cannot be created.
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, nil, 0o644)
	if _, _, err := Setup(Options{Level: "info", File: filepath.Join(blocker, "server.log")}); err == nil {
		t.Error("an unusable log path should fail")
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateMigration_NumbersSequentiallyAndSanitizesName(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"00001_a.sql", "00007_b.sql", "notes.txt"} {
		os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644)
	}

	path, err := createMigration(dir, "Add Failed-At column!")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "00008_add_failed_at_column.sql" {
		t.Fatalf("path = %s, want 00008_add_failed_at_column.sql (one past the highest, 7)", path)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "-- +goose Up") || !strings.Contains(string(body), "-- +goose Down") {
		t.Fatalf("template lacks Up/Down markers:\n%s", body)
	}

	path2, _ := createMigration(dir, "next")
	if filepath.Base(path2) != "00009_next.sql" {
		t.Fatalf("second file = %s", path2)
	}
}

func TestCreateMigration_Errors(t *testing.T) {
	if _, err := createMigration(t.TempDir(), "!!!"); err == nil {
		t.Error("a name with no letters or digits must be rejected")
	}
	if _, err := createMigration(filepath.Join(t.TempDir(), "missing"), "x"); err == nil {
		t.Error("a missing directory must be an error")
	}
}

func TestSplitArgs_FlagsWorkInAnyPosition(t *testing.T) {
	for _, in := range [][]string{{"1", "-yes"}, {"-yes", "1"}} {
		flags, words := splitArgs(in)
		if len(flags) != 1 || flags[0] != "-yes" || len(words) != 1 || words[0] != "1" {
			t.Errorf("splitArgs(%v) = %v, %v", in, flags, words)
		}
	}
}

func TestVersionArg(t *testing.T) {
	if v, err := versionArg("up-to", []string{"3"}); err != nil || v != 3 {
		t.Errorf("valid: %d, %v", v, err)
	}
	if v, err := versionArg("down-to", []string{"0"}); err != nil || v != 0 {
		t.Errorf("zero is valid for down-to: %d, %v", v, err)
	}
	for _, bad := range [][]string{nil, {"abc"}, {"-1"}, {"1", "2"}} {
		if _, err := versionArg("up-to", bad); err == nil {
			t.Errorf("versionArg(%v) should fail", bad)
		}
	}
}

package recipients

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Concept: a table-driven test. Each row of the table is one scenario; the
// loop runs them all. Adding a new case is one new line, which is why this
// is the idiomatic Go testing style.
//
// The inputs are the real .xlsx files in /samples, so the tests double as
// documentation of what each sample file is for.
func TestParseExcel(t *testing.T) {
	tests := []struct {
		file         string
		wantValid    int
		wantProblems int
	}{
		{"1_valid.xlsx", 5, 0},
		{"2_missing_emails.xlsx", 3, 2},
		{"3_invalid_emails.xlsx", 3, 5}, // gina@example passes: we only check format
		{"4_missing_names.xlsx", 2, 2},
		{"5_duplicates_and_whitespace.xlsx", 3, 2},
		{"6_blank_rows.xlsx", 3, 0},
		{"8_headers_only.xlsx", 0, 0},
		{"9_mixed_everything.xlsx", 3, 5},
	}

	for _, tt := range tests {
		// t.Run gives each scenario its own name in the test output.
		t.Run(tt.file, func(t *testing.T) {
			valid, problems, err := ParseExcel(filepath.Join("..", "..", "samples", tt.file))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(valid) != tt.wantValid {
				t.Errorf("valid = %d, want %d (%v)", len(valid), tt.wantValid, valid)
			}
			if len(problems) != tt.wantProblems {
				t.Errorf("problems = %d, want %d (%v)", len(problems), tt.wantProblems, problems)
			}
		})
	}
}

// Whitespace and case are normalised, and the FIRST duplicate wins.
func TestParseExcelNormalises(t *testing.T) {
	valid, _, err := ParseExcel(filepath.Join("..", "..", "samples", "5_duplicates_and_whitespace.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	want := Recipient{Name: "Ben Carter", Email: "ben@example.com"}
	if valid[1] != want {
		t.Errorf("valid[1] = %+v, want %+v", valid[1], want)
	}
}

// A file-level problem comes back as an error, not as row errors.
func TestParseExcelWrongHeaders(t *testing.T) {
	_, _, err := ParseExcel(filepath.Join("..", "..", "samples", "7_wrong_headers.xlsx"))
	if !errors.Is(err, ErrMissingColumns) {
		t.Errorf("err = %v, want ErrMissingColumns", err)
	}
}

func TestParseExcelMissingFile(t *testing.T) {
	if _, _, err := ParseExcel("does-not-exist.xlsx"); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestParseExcelReader(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "samples", "1_valid.xlsx"))
	if err != nil {
		t.Fatalf("open sample file: %v", err)
	}
	defer f.Close()

	valid, problems, err := ParseExcelReader(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(valid) != 5 {
		t.Errorf("valid = %d, want 5", len(valid))
	}
	if len(problems) != 0 {
		t.Errorf("problems = %d, want 0", len(problems))
	}
}


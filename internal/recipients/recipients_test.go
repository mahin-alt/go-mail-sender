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
	got := valid[1]
	if got.Name != "Ben Carter" || got.Email != "ben@example.com" {
		t.Errorf("valid[1] = %+v, want Ben Carter <ben@example.com>", got)
	}
	if got.Row < 2 {
		t.Errorf("valid[1].Row = %d, want a data row (>= 2)", got.Row)
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


// TestScanKinds checks the kind and row number Scan reports for every row of
// the "everything wrong" sample, including the row numbers after a blank row.
func TestScanKinds(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "samples", "9_mixed_everything.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	type got struct {
		Num  int
		Kind Kind
	}
	var rows []got
	if err := Scan(f, func(r Row) error {
		if r.Kind != KindOK && r.Reason == "" {
			t.Errorf("row %d (%s) has no reason", r.Num, r.Kind)
		}
		rows = append(rows, got{r.Num, r.Kind})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	want := []got{
		{2, KindOK}, {3, KindEmptyEmail}, {4, KindInvalidEmail}, {5, KindEmptyName},
		{6, KindOK}, {8, KindDuplicate}, {9, KindInvalidEmail}, {10, KindOK}, // row 7 is blank
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows %+v, want %d", len(rows), rows, len(want))
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d: got %+v, want %+v", i, rows[i], want[i])
		}
	}
}

// TestScanStopsOnCallbackError checks that an error from the callback (for
// example a failed database write) stops the scan and is returned as is.
func TestScanStopsOnCallbackError(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "samples", "1_valid.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	stop := errors.New("stop")
	calls := 0
	err = Scan(f, func(Row) error { calls++; return stop })
	if !errors.Is(err, stop) || calls != 1 {
		t.Fatalf("err = %v after %d calls, want the callback's error after 1 call", err, calls)
	}
}

func TestKindSendable(t *testing.T) {
	for _, k := range []Kind{KindOK, KindUnverified} {
		if !k.Sendable() {
			t.Errorf("%s should be sendable", k)
		}
	}
	for _, k := range SkippedKinds {
		if k.Sendable() {
			t.Errorf("%s should be skipped", k)
		}
	}
}

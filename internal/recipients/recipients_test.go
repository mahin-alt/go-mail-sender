package recipients

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scanSample runs Scan over one of the files in /samples and returns every
// row it reported.
func scanSample(t *testing.T, name string) ([]Row, error) {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "samples", name))
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}
	defer f.Close()
	var rows []Row
	err = Scan(f, func(r Row) error {
		rows = append(rows, r)
		return nil
	})
	return rows, err
}

// Concept: a table-driven test. Each row of the table is one scenario; the
// loop runs them all. The inputs are the real .xlsx files in /samples, so the
// tests double as documentation of what each sample file is for.
func TestScanSamples(t *testing.T) {
	tests := []struct {
		file         string
		wantValid    int
		wantProblems int
	}{
		{"1_valid.xlsx", 5, 0},
		{"2_missing_emails.xlsx", 3, 2},
		{"3_invalid_emails.xlsx", 3, 5}, // gina@example passes: we only check format
		{"4_missing_names.xlsx", 2, 2},
		{"5_duplicates_and_whitespace.xlsx", 5, 0}, // duplicates are sent, not flagged
		{"6_blank_rows.xlsx", 3, 0},
		{"8_headers_only.xlsx", 0, 0},
		{"9_mixed_everything.xlsx", 4, 4},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			rows, err := scanSample(t, tt.file)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			valid := 0
			for _, r := range rows {
				if r.Kind.Sendable() {
					valid++
				}
			}
			if valid != tt.wantValid || len(rows)-valid != tt.wantProblems {
				t.Errorf("valid = %d, problems = %d, want %d and %d (%+v)", valid, len(rows)-valid, tt.wantValid, tt.wantProblems, rows)
			}
		})
	}
}

// Whitespace and case are normalised, and a repeated email is an ordinary row.
func TestScanNormalisesAndKeepsDuplicates(t *testing.T) {
	rows, err := scanSample(t, "5_duplicates_and_whitespace.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, r := range rows {
		if r.Email != strings.ToLower(strings.TrimSpace(r.Email)) || r.Name != strings.TrimSpace(r.Name) {
			t.Errorf("row %d not normalised: %q <%q>", r.Num, r.Name, r.Email)
		}
		seen[r.Email]++
	}
	if seen["ben@example.com"] == 0 {
		t.Errorf("rows = %+v, want Ben Carter's padded, upper-case address as ben@example.com", rows)
	}
	dups := 0
	for _, n := range seen {
		if n > 1 {
			dups++
		}
	}
	if dups == 0 {
		t.Error("the sample has repeated emails; both copies must be reported as OK rows")
	}
}

// A file-level problem comes back as an error, not as row errors.
func TestScanWrongHeaders(t *testing.T) {
	if _, err := scanSample(t, "7_wrong_headers.xlsx"); !errors.Is(err, ErrMissingColumns) {
		t.Errorf("err = %v, want ErrMissingColumns", err)
	}
}

func TestScanNotASpreadsheet(t *testing.T) {
	if err := Scan(strings.NewReader("hello"), func(Row) error { return nil }); err == nil {
		t.Error("expected an error for a file that is not .xlsx")
	}
}

// TestScanKinds checks the kind and row number Scan reports for every row of
// the "everything wrong" sample, including the row numbers after a blank row.
func TestScanKinds(t *testing.T) {
	rows, err := scanSample(t, "9_mixed_everything.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	type got struct {
		Num  int
		Kind Kind
	}
	want := []got{
		{2, KindOK}, {3, KindEmptyEmail}, {4, KindInvalidEmail}, {5, KindEmptyName},
		{6, KindOK}, {8, KindOK}, {9, KindInvalidEmail}, {10, KindOK}, // row 7 is blank; row 8 repeats row 2's email
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows %+v, want %d", len(rows), rows, len(want))
	}
	for i, r := range rows {
		if (got{r.Num, r.Kind}) != want[i] {
			t.Errorf("row %d: got %d %s, want %+v", i, r.Num, r.Kind, want[i])
		}
		if r.Kind != KindOK && r.Reason == "" {
			t.Errorf("row %d (%s) has no reason", r.Num, r.Kind)
		}
	}
}

func TestCheckRowLengthCaps(t *testing.T) {
	long := strings.Repeat("a", 300) + "@example.com"
	r := checkRow(2, strings.Repeat("N", 400), long)
	if r.Kind != KindInvalidEmail {
		t.Errorf("a %d-character address must be invalid, got %s", len(long), r.Kind)
	}
	if len([]rune(r.Name)) != maxNameLen {
		t.Errorf("name kept %d characters, want %d", len([]rune(r.Name)), maxNameLen)
	}
	huge := checkRow(2, "N", strings.Repeat("é", 2000))
	if n := len([]rune(huge.Email)); n != maxStoreLen {
		t.Errorf("stored email has %d characters, want it cut to %d", n, maxStoreLen)
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
	for _, k := range []Kind{KindEmptyEmail, KindEmptyName, KindInvalidEmail, KindBadDomain} {
		if k.Sendable() {
			t.Errorf("%s should be skipped", k)
		}
	}
}

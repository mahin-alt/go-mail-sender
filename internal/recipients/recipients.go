// Package recipients reads a list of people to email from an Excel file.
//
// Concept: like the mailer package, this one knows nothing about HTTP or SMTP.
// It only answers one question: "given this .xlsx file, who should we email,
// and which rows were unusable?". Keeping it separate means we can test it
// with plain files, without sending any mail.
//
// Expected file layout: the FIRST sheet, with a header row containing
// "name" and "email" columns (any order, any capitalisation), then one
// person per row:
//
//	| name         | email             |
//	| Aisha Rahman | aisha@example.com |
package recipients

import (
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Recipient is one person we will send an email to.
type Recipient struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Row   int    `json:"row"` // spreadsheet row (header is row 1), so later checks can point back at it
}

// RowError describes one row we could not use. It is DATA, not a failure of
// the whole run: a bad row should not stop the good rows from being sent.
type RowError struct {
	Row    int    `json:"row"`             // the row number as shown in Excel (header is row 1)
	Name   string `json:"name,omitempty"`  // raw name from spreadsheet
	Email  string `json:"email,omitempty"` // raw email from spreadsheet
	Reason string `json:"reason"`          // human readable explanation
}

// Error makes RowError satisfy Go's built-in `error` interface (any type with
// an `Error() string` method is an error). We don't strictly need that here,
// but it lets callers print a RowError or wrap it like any other error.
func (e RowError) Error() string {
	return fmt.Sprintf("row %d: %s", e.Row, e.Reason)
}

// ErrMissingColumns is a "sentinel error": a package-level value callers can
// test for with errors.Is. It means the file is the wrong shape entirely
// (no name/email header), as opposed to a single bad row.
var ErrMissingColumns = errors.New(`sheet must have "name" and "email" columns in its header row`)

// ParseExcel reads path and returns the usable recipients plus the rows that
// were rejected.
//
// Concept: two kinds of problems are reported in two different ways.
//   - Problems with the FILE (can't open it, wrong headers) -> the returned `error`.
//     Nothing can be done, so the caller stops.
//   - Problems with a ROW (empty email, bad format, duplicate) -> the []RowError.
//     The caller can still send to the valid recipients and show the user the rest.
func ParseExcel(path string) ([]Recipient, []RowError, error) {
	// OpenFile reads and unzips the .xlsx (an .xlsx is a zip of XML files).
	f, err := excelize.OpenFile(path)
	if err != nil {
		// %w wraps the original error so callers can still inspect it with
		// errors.Is / errors.As, while we add context about what we were doing.
		return nil, nil, fmt.Errorf("open excel file: %w", err)
	}
	defer f.Close()

	return parseFile(f)
}

// ParseExcelReader reads an Excel document from r and returns the usable recipients
// plus the rows that were rejected.
func ParseExcelReader(r io.Reader) ([]Recipient, []RowError, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("open excel file: %w", err)
	}
	defer f.Close()

	return parseFile(f)
}

func parseFile(f *excelize.File) ([]Recipient, []RowError, error) {
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, nil, errors.New("excel file has no sheets")
	}

	// GetRows returns every row of the sheet as a slice of strings. Every cell
	// comes back as text (numbers, dates and formula results included), which
	// suits us because names and emails are text anyway.
	// Note: Excel drops trailing empty cells, so rows can have different lengths.
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, nil, fmt.Errorf("read sheet %q: %w", sheets[0], err)
	}
	if len(rows) == 0 {
		return nil, nil, ErrMissingColumns
	}

	nameCol, emailCol := findColumns(rows[0])
	if nameCol < 0 || emailCol < 0 {
		return nil, nil, ErrMissingColumns
	}

	valid := make([]Recipient, 0)
	problems := make([]RowError, 0)
	seen := make(map[string]struct{})

	// rows[0] is the header, so data starts at index 1.
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		rowNum := i + 1 // slice index 1 is spreadsheet row 2

		name := cell(row, nameCol)
		email := cell(row, emailCol)

		// A fully empty row (common at the bottom of a sheet) is not an error.
		if name == "" && email == "" {
			continue
		}
		if email == "" {
			problems = append(problems, RowError{Row: rowNum, Name: name, Email: email, Reason: "email is empty"})
			continue
		}
		if name == "" {
			// Policy choice: we require a name. If you'd rather email people
			// without one, remove this check and use a fallback greeting.
			problems = append(problems, RowError{Row: rowNum, Name: name, Email: email, Reason: "name is empty"})
			continue
		}
		if !validEmail(email) {
			problems = append(problems, RowError{Row: rowNum, Name: name, Email: email, Reason: fmt.Sprintf("invalid email %q", email)})
			continue
		}

		// Email addresses are case-insensitive in practice, so compare lowercase.
		email = strings.ToLower(email)
		if _, dup := seen[email]; dup {
			problems = append(problems, RowError{Row: rowNum, Name: name, Email: email, Reason: fmt.Sprintf("duplicate email %q", email)})
			continue
		}
		seen[email] = struct{}{}

		valid = append(valid, Recipient{Name: name, Email: email, Row: rowNum})
	}

	return valid, problems, nil
}

// findColumns returns the index of the "name" and "email" columns in the
// header row, or -1 for any that is missing.
func findColumns(header []string) (nameCol, emailCol int) {
	nameCol, emailCol = -1, -1
	for i, h := range header {
		// EqualFold is a case-insensitive compare ("Email" == "EMAIL").
		switch h = strings.TrimSpace(h); {
		case strings.EqualFold(h, "name"):
			nameCol = i
		case strings.EqualFold(h, "email"):
			emailCol = i
		}
	}
	return nameCol, emailCol
}

// cell safely returns the trimmed text at index i. Because Excel drops
// trailing empty cells, a short row would make row[i] panic with
// "index out of range" - so we check the length first.
func cell(row []string, i int) string {
	if i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// validEmail reports whether s is a single, plain email address.
//
// We use the standard library's net/mail parser rather than a hand-written
// regex (email syntax is notoriously hard to get right). One catch:
// mail.ParseAddress also accepts display-name forms like "Bob <bob@x.com>",
// so we require that the parsed address equals the input exactly.
//
// This only checks FORMAT. It cannot know whether the mailbox exists, and it
// accepts dotless domains such as "gina@example".
func validEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}

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

// ErrMissingColumns is a "sentinel error": a package-level value callers can
// test for with errors.Is. It means the file is the wrong shape entirely
// (no name/email header), as opposed to a single bad row.
var ErrMissingColumns = errors.New(`sheet must have "name" and "email" columns in its header row`)

// Kind is the result of checking one row. KindOK and KindUnverified rows are
// sent; every other kind is skipped. The skipped kinds are stored as
// email_recipients.validation_reason and returned by the API, so do not
// rename them. Duplicate emails are NOT flagged: each row gets its email.
type Kind string

const (
	KindOK           Kind = "ok"            // valid, will be sent
	KindUnverified   Kind = "unverified"    // looks valid, domain could not be checked; still sent
	KindEmptyEmail   Kind = "empty_email"   // name present, email empty
	KindEmptyName    Kind = "empty_name"    // email present, name empty
	KindInvalidEmail Kind = "invalid_email" // not a plain email address
	KindBadDomain    Kind = "bad_domain"    // the domain cannot receive mail
)

// Length caps. Excel cells can hold 32,767 characters; nothing that long is a
// real name or address, so longer values are cut before they are stored.
const (
	maxNameLen  = 255
	maxEmailLen = 254 // the longest address SMTP allows
	maxStoreLen = 500 // an over-long (invalid) email is still shown, cut to this
)

// Sendable reports whether rows of this kind are sent.
func (k Kind) Sendable() bool { return k == KindOK || k == KindUnverified }

// Row is one non-empty data row and what the check found.
type Row struct {
	Num    int    // spreadsheet row (header is row 1)
	Name   string // trimmed
	Email  string // trimmed; lowercased for valid rows
	Kind   Kind
	Reason string // human readable, empty for KindOK
}

// Scan reads an Excel document from r and calls fn once for every non-empty
// data row, in spreadsheet order, with the row's syntax check already done
// (KindOK, KindEmptyEmail, KindEmptyName or KindInvalidEmail; domain checks
// are the caller's job).
//
// Concept: streaming. Rows are read one at a time (excelize's Rows iterator)
// instead of loading the whole sheet first (GetRows), and nothing is kept
// per row, so memory does not grow with the number of rows. (excelize still
// holds the compressed file and its shared-strings table while reading.)
// If fn returns an error, Scan stops and returns it.
func Scan(r io.Reader, fn func(Row) error) error {
	// OpenReader unzips the .xlsx (a zip of XML files). Big sheets are unpacked
	// to a temporary file rather than memory (excelize's UnzipXMLSizeLimit).
	f, err := excelize.OpenReader(r)
	if err != nil {
		return fmt.Errorf("open excel file: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return errors.New("excel file has no sheets")
	}
	rows, err := f.Rows(sheets[0])
	if err != nil {
		return fmt.Errorf("read sheet %q: %w", sheets[0], err)
	}
	defer rows.Close()

	// Next moves one spreadsheet row at a time, empty rows included, so
	// counting calls gives the row number (the header must be row 1).
	if !rows.Next() {
		if err := rows.Error(); err != nil {
			return fmt.Errorf("read sheet %q: %w", sheets[0], err)
		}
		return ErrMissingColumns
	}
	header, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("read header row: %w", err)
	}
	nameCol, emailCol := findColumns(header)
	if nameCol < 0 || emailCol < 0 {
		return ErrMissingColumns
	}

	for rowNum := 2; rows.Next(); rowNum++ {
		row, err := rows.Columns()
		if err != nil {
			return fmt.Errorf("read row %d: %w", rowNum, err)
		}
		r := checkRow(rowNum, cell(row, nameCol), cell(row, emailCol))
		if r.Kind == "" { // fully empty row (common at the bottom of a sheet): not an error
			continue
		}
		if err := fn(r); err != nil {
			return err
		}
	}
	if err := rows.Error(); err != nil {
		return fmt.Errorf("read sheet %q: %w", sheets[0], err)
	}
	return nil
}

// checkRow applies the per-row rules. A row with no name and no email comes
// back with an empty Kind, meaning "skip silently".
func checkRow(rowNum int, name, email string) Row {
	name, email = truncate(name, maxNameLen), truncate(email, maxStoreLen)
	r := Row{Num: rowNum, Name: name, Email: email}
	switch {
	case name == "" && email == "":
		return Row{}
	case email == "":
		r.Kind, r.Reason = KindEmptyEmail, "email is empty"
	case name == "":
		// Policy choice: we require a name. If you'd rather email people
		// without one, remove this check and use a fallback greeting.
		r.Kind, r.Reason = KindEmptyName, "name is empty"
	case len(email) > maxEmailLen || !validEmail(email):
		r.Kind, r.Reason = KindInvalidEmail, fmt.Sprintf("invalid email %q", email)
	default:
		// Email addresses are case-insensitive in practice; store them lowercase.
		r.Email = strings.ToLower(email)
		r.Kind = KindOK
	}
	return r
}

// truncate cuts s to at most n characters (runes, so a letter is never split).
func truncate(s string, n int) string {
	if len(s) <= n { // bytes >= runes, so this is a cheap common-case exit
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
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

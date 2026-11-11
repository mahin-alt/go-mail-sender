package store

import (
	"context"
	"fmt"
)

// Recipient validation statuses (email_recipients.validation_status).
const (
	RecipientValid      = "valid"      // will be sent
	RecipientUnverified = "unverified" // looks fine, domain could not be checked; still sent
	RecipientInvalid    = "invalid"    // skipped; validation_reason says why
)

// Recipient is one saved spreadsheet row.
type Recipient struct {
	ID               int64   `json:"id"`
	Row              int     `json:"row"`
	Name             string  `json:"name"`
	Email            string  `json:"email"`
	ValidationStatus string  `json:"validation_status"`
	ValidationReason *string `json:"validation_reason"`
}

// RecipientFilter selects one page of a batch's rows.
//
// Concept: keyset ("cursor") paging. Instead of OFFSET (skip N rows, which
// gets slower the deeper you page), the next page starts "after ID X", and
// the index on (batch_id, ..., id) jumps straight there. Rows are saved in
// spreadsheet order, so ID order is row order.
type RecipientFilter struct {
	// Status is RecipientInvalid, RecipientUnverified or RecipientValid, or
	// Sendable for valid + unverified rows (the rows that get an email).
	Status  string
	Reasons []string // only invalid rows with one of these reasons, empty = any
	Search  string   // case-insensitive substring of name or email, "" = any
	After   int64    // only rows with a larger ID (0 = from the start)
	Limit   int
}

// Sendable is the RecipientFilter status for valid + unverified rows.
const Sendable = "sendable"

// ListRecipients returns up to f.Limit rows of a batch in spreadsheet order.
// It powers the Issues tabs (invalid / unverified rows) and the preview
// sample (the first sendable rows).
func (s *Store) ListRecipients(ctx context.Context, batchID int64, f RecipientFilter) ([]Recipient, error) {
	// The status condition is written out (not passed as a parameter) so the
	// planner can match it to the right index, e.g. the partial "sendable" one.
	var statusCond string
	switch f.Status {
	case Sendable:
		statusCond = `validation_status <> 'invalid'`
	case RecipientValid, RecipientUnverified, RecipientInvalid:
		statusCond = `validation_status = '` + f.Status + `'`
	default:
		return nil, fmt.Errorf("unknown recipient status %q", f.Status)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, row_number, name, email, validation_status, validation_reason
		FROM email_recipients
		WHERE batch_id = $1
		  AND `+statusCond+`
		  AND (cardinality($2::text[]) = 0 OR validation_reason = ANY($2))
		  AND id > $3
		  AND ($4 = '' OR email ILIKE '%' || $4 || '%' OR name ILIKE '%' || $4 || '%')
		ORDER BY id
		LIMIT $5`,
		batchID, append([]string{}, f.Reasons...), f.After, f.Search, f.Limit)
	if err != nil {
		return nil, fmt.Errorf("list recipients: %w", err)
	}
	defer rows.Close()

	out := []Recipient{} // non-nil so it encodes as [] not null
	for rows.Next() {
		var r Recipient
		if err := rows.Scan(&r.ID, &r.Row, &r.Name, &r.Email, &r.ValidationStatus, &r.ValidationReason); err != nil {
			return nil, fmt.Errorf("scan recipient: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recipient rows: %w", err)
	}
	return out, nil
}

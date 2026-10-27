package store

import (
	"context"
	"fmt"
	"time"
)

// attentionWindow is how far back batches are checked for a high share of
// invalid rows. Batches with failed jobs are shown regardless of age, until
// those jobs are retried or sent.
const attentionWindow = 30 * 24 * time.Hour

// StatsOptions configures GetStats.
type StatsOptions struct {
	Days     int            // length of the daily series, ending today
	Location *time.Location // decides where "today" and each day boundary fall
}

// TodayStats counts what happened since local midnight.
type TodayStats struct {
	Sent   int `json:"sent"`
	Failed int `json:"failed"` // jobs that permanently failed today
}

// QueueStats is a snapshot of work still to do.
type QueueStats struct {
	Pending    int `json:"pending"`    // waiting to be sent, including retries
	Retrying   int `json:"retrying"`   // the part of pending that failed at least once and is waiting for another try
	Processing int `json:"processing"` // being sent right now
}

// DayStat is one point of the daily chart.
type DayStat struct {
	Date   string `json:"date"` // YYYY-MM-DD in the requested time zone
	Sent   int    `json:"sent"`
	Failed int    `json:"failed"`
}

// AttentionBatch is a batch that probably needs a human look.
type AttentionBatch struct {
	BatchID   int64     `json:"batch_id"`
	Filename  string    `json:"filename"`
	Status    string    `json:"status"`
	Total     int       `json:"total"`
	Failed    int       `json:"failed"`
	Invalid   int       `json:"invalid"`
	CreatedAt time.Time `json:"created_at"`
	Reasons   []string  `json:"reasons"` // "failed_jobs" and/or "high_invalid_rate"
}

// Stats is everything the Overview page shows.
type Stats struct {
	GeneratedAt    time.Time        `json:"generated_at"`
	Timezone       string           `json:"timezone"`
	Today          TodayStats       `json:"today"`
	Queue          QueueStats       `json:"queue"`
	FailedTotal    int              `json:"failed_total"`   // failed jobs still unresolved, across all batches
	ActiveBatches  int              `json:"active_batches"` // batches that are pending or processing
	Daily          []DayStat        `json:"daily"`
	NeedsAttention []AttentionBatch `json:"needs_attention"`
}

// highInvalidRate: a batch is flagged when at least 1 in 5 of its rows is invalid.
func highInvalidRate(invalid, total int) bool {
	return invalid > 0 && invalid*5 >= total
}

// GetStats gathers the dashboard Overview numbers.
//
// Each count is a separate small query that uses its own partial index, so
// the cost stays low as the email_jobs table grows (a single query with
// several FILTER aggregates would have to scan the whole table).
func (s *Store) GetStats(ctx context.Context, opt StatsOptions) (*Stats, error) {
	loc := opt.Location
	if loc == nil {
		loc = time.UTC
	}
	days := opt.Days
	if days < 1 {
		days = 14
	}

	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	windowStart := todayStart.AddDate(0, 0, -(days - 1))
	tz := loc.String()

	st := &Stats{GeneratedAt: now.UTC(), Timezone: tz}

	// 1. Tiles: one round trip, each scalar subquery served by its own index.
	err := s.db.QueryRowContext(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM email_jobs WHERE status = 'sent'   AND sent_at    >= $1),
		  (SELECT COUNT(*) FROM email_jobs WHERE status = 'failed' AND updated_at >= $1),
		  (SELECT COUNT(*) FROM email_jobs WHERE status = 'pending'),
		  (SELECT COUNT(*) FROM email_jobs WHERE status = 'pending' AND attempts > 0),
		  (SELECT COUNT(*) FROM email_jobs WHERE status = 'processing'),
		  (SELECT COUNT(*) FROM email_jobs WHERE status = 'failed'),
		  (SELECT COUNT(*) FROM batches    WHERE status IN ('pending', 'processing'))
	`, todayStart).Scan(
		&st.Today.Sent, &st.Today.Failed,
		&st.Queue.Pending, &st.Queue.Retrying, &st.Queue.Processing,
		&st.FailedTotal, &st.ActiveBatches,
	)
	if err != nil {
		return nil, fmt.Errorf("stats tiles: %w", err)
	}

	// 2. Daily series: group by calendar day in the requested time zone, then
	// fill days with no activity with zeros so the chart has no gaps.
	byDay := map[string]*DayStat{}
	st.Daily = make([]DayStat, days)
	for i := range st.Daily {
		d := windowStart.AddDate(0, 0, i).Format("2006-01-02")
		st.Daily[i] = DayStat{Date: d}
		byDay[d] = &st.Daily[i]
	}
	for _, q := range []struct {
		sql  string
		into func(*DayStat, int)
	}{
		{`SELECT to_char(sent_at AT TIME ZONE $2, 'YYYY-MM-DD'), COUNT(*)
		  FROM email_jobs WHERE status = 'sent' AND sent_at >= $1 GROUP BY 1`,
			func(d *DayStat, n int) { d.Sent = n }},
		{`SELECT to_char(updated_at AT TIME ZONE $2, 'YYYY-MM-DD'), COUNT(*)
		  FROM email_jobs WHERE status = 'failed' AND updated_at >= $1 GROUP BY 1`,
			func(d *DayStat, n int) { d.Failed = n }},
	} {
		rows, err := s.db.QueryContext(ctx, q.sql, windowStart, tz)
		if err != nil {
			return nil, fmt.Errorf("stats daily: %w", err)
		}
		for rows.Next() {
			var day string
			var n int
			if err := rows.Scan(&day, &n); err != nil {
				rows.Close()
				return nil, fmt.Errorf("stats daily scan: %w", err)
			}
			if d, ok := byDay[day]; ok {
				q.into(d, n)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("stats daily rows: %w", err)
		}
	}

	// 3. Needs attention: batches with unresolved failed jobs, or (recent
	// batches only) a high share of invalid rows. Newest first, at most 10.
	rows, err := s.db.QueryContext(ctx, `
		WITH failed AS (
		  SELECT batch_id, COUNT(*) AS n FROM email_jobs WHERE status = 'failed' GROUP BY batch_id
		)
		SELECT b.id, b.filename, b.status, b.total_jobs, COALESCE(f.n, 0), COALESCE(inv.n, 0), b.created_at
		FROM batches b
		LEFT JOIN failed f ON f.batch_id = b.id
		LEFT JOIN LATERAL (
		  SELECT COUNT(*) AS n FROM email_jobs j
		  WHERE j.batch_id = b.id AND j.status = 'invalid' AND b.created_at >= $1
		) inv ON TRUE
		WHERE COALESCE(f.n, 0) > 0
		   OR (COALESCE(inv.n, 0) > 0 AND COALESCE(inv.n, 0) * 5 >= b.total_jobs)
		ORDER BY b.created_at DESC, b.id DESC
		LIMIT 10
	`, time.Now().Add(-attentionWindow))
	if err != nil {
		return nil, fmt.Errorf("stats attention: %w", err)
	}
	defer rows.Close()

	st.NeedsAttention = []AttentionBatch{} // non-nil so it encodes as [] not null
	for rows.Next() {
		var a AttentionBatch
		if err := rows.Scan(&a.BatchID, &a.Filename, &a.Status, &a.Total, &a.Failed, &a.Invalid, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("stats attention scan: %w", err)
		}
		a.Reasons = []string{}
		if a.Failed > 0 {
			a.Reasons = append(a.Reasons, "failed_jobs")
		}
		if highInvalidRate(a.Invalid, a.Total) {
			a.Reasons = append(a.Reasons, "high_invalid_rate")
		}
		st.NeedsAttention = append(st.NeedsAttention, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("stats attention rows: %w", err)
	}
	return st, nil
}

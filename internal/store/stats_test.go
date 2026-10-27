package store

import (
	"context"
	"testing"
	"time"
)

// wipe removes all batches (and their jobs, by cascade) so stats start from zero.
func wipe(t *testing.T, st *Store) {
	t.Helper()
	if _, err := st.db.Exec(`DELETE FROM batches`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.db.Exec(`DELETE FROM batches`) })
}

func setJob(t *testing.T, st *Store, id int64, sentAt, updatedAt *time.Time, attempts int) {
	t.Helper()
	if _, err := st.db.Exec(`UPDATE email_jobs SET sent_at = $2, updated_at = COALESCE($3, updated_at), attempts = $4 WHERE id = $1`,
		id, sentAt, updatedAt, attempts); err != nil {
		t.Fatal(err)
	}
}

func backdateSent(t *testing.T, st *Store, batchID int64, at time.Time) {
	t.Helper()
	if _, err := st.db.Exec(`UPDATE email_jobs SET sent_at = $2 WHERE batch_id = $1 AND status = 'sent'`, batchID, at); err != nil {
		t.Fatal(err)
	}
}

func daily(s *Stats, date string) DayStat {
	for _, d := range s.Daily {
		if d.Date == date {
			return d
		}
	}
	return DayStat{Date: "MISSING"}
}

func TestHighInvalidRate(t *testing.T) {
	for _, c := range []struct {
		invalid, total int
		want           bool
	}{
		{0, 10, false}, {1, 10, false}, {2, 10, true}, {3, 10, true}, {1, 5, true}, {0, 0, false}, {1, 100, false},
	} {
		if got := highInvalidRate(c.invalid, c.total); got != c.want {
			t.Errorf("highInvalidRate(%d,%d) = %v, want %v", c.invalid, c.total, got, c.want)
		}
	}
}

func TestGetStats_EmptyDatabase(t *testing.T) {
	st := openTestStore(t)
	wipe(t, st)

	s, err := st.GetStats(context.Background(), StatsOptions{Days: 7})
	if err != nil {
		t.Fatal(err)
	}
	if s.Today != (TodayStats{}) || s.Queue != (QueueStats{}) || s.FailedTotal != 0 || s.ActiveBatches != 0 {
		t.Fatalf("expected all zeros: %+v", s)
	}
	if len(s.Daily) != 7 {
		t.Fatalf("daily has %d points, want 7 (zero-filled)", len(s.Daily))
	}
	for _, d := range s.Daily {
		if d.Sent != 0 || d.Failed != 0 {
			t.Fatalf("empty db must give an all-zero series: %+v", d)
		}
	}
	if s.NeedsAttention == nil || len(s.NeedsAttention) != 0 {
		t.Fatalf("needs_attention must be an empty non-nil slice, got %#v", s.NeedsAttention)
	}
}

func TestGetStats_KnownData(t *testing.T) {
	st := openTestStore(t)
	wipe(t, st)
	ctx := context.Background()

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	day := func(offset int) time.Time { return today.AddDate(0, 0, offset) }

	// Batch A: 9 jobs in assorted states. Expected contribution noted per line.
	aID, a := seedBatch(t, st, "sent", "sent", "sent", "failed", "failed", "pending", "pending", "processing", "invalid")
	at := func(t time.Time) *time.Time { return &t }
	setJob(t, st, a[0], at(day(0).Add(time.Minute)), nil, 1)   // sent today
	setJob(t, st, a[1], at(day(0).Add(-time.Hour)), nil, 1)    // sent yesterday 23:00
	setJob(t, st, a[2], at(day(-3).Add(12*time.Hour)), nil, 1) // sent 3 days ago
	setJob(t, st, a[3], nil, at(day(0).Add(time.Minute)), 3)   // failed today
	setJob(t, st, a[4], nil, at(day(-1).Add(12*time.Hour)), 3) // failed yesterday
	setJob(t, st, a[5], nil, nil, 0)                           // pending, first try
	setJob(t, st, a[6], nil, nil, 1)                           // pending, retrying
	_ = aID

	// Batch B: 5 sent 20 days ago -> outside a 14-day window.
	bID, _ := seedBatch(t, st, "sent", "sent", "sent", "sent", "sent")
	backdateSent(t, st, bID, day(-20))

	// Batch C: 10 rows, 3 invalid (30%) -> flagged high_invalid_rate.
	cID, _ := seedBatch(t, st, "sent", "sent", "sent", "sent", "sent", "sent", "sent", "invalid", "invalid", "invalid")
	backdateSent(t, st, cID, day(-20))

	// Batch D: 10 rows, 1 invalid (10%) -> not flagged.
	dID, _ := seedBatch(t, st, "sent", "sent", "sent", "sent", "sent", "sent", "sent", "sent", "sent", "invalid")
	backdateSent(t, st, dID, day(-20))

	// Batch E: all invalid but created 60 days ago -> outside the attention window.
	eID, _ := seedBatch(t, st, "invalid", "invalid", "invalid", "invalid", "invalid")
	if _, err := st.db.Exec(`UPDATE batches SET created_at = $2 WHERE id = $1`, eID, day(-60)); err != nil {
		t.Fatal(err)
	}

	s, err := st.GetStats(ctx, StatsOptions{Days: 14, Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}

	if s.Timezone != "UTC" {
		t.Errorf("timezone = %q", s.Timezone)
	}
	if s.Today.Sent != 1 || s.Today.Failed != 1 {
		t.Errorf("today = %+v, want sent 1, failed 1", s.Today)
	}
	if s.Queue.Pending != 2 || s.Queue.Retrying != 1 || s.Queue.Processing != 1 {
		t.Errorf("queue = %+v, want pending 2, retrying 1, processing 1", s.Queue)
	}
	if s.FailedTotal != 2 {
		t.Errorf("failed_total = %d, want 2", s.FailedTotal)
	}
	if s.ActiveBatches != 1 {
		t.Errorf("active_batches = %d, want 1 (only batch A has pending work)", s.ActiveBatches)
	}

	if len(s.Daily) != 14 || s.Daily[13].Date != day(0).Format("2006-01-02") || s.Daily[0].Date != day(-13).Format("2006-01-02") {
		t.Fatalf("daily window wrong: first=%s last=%s len=%d", s.Daily[0].Date, s.Daily[13].Date, len(s.Daily))
	}
	for offset, want := range map[int]DayStat{
		0:  {Sent: 1, Failed: 1},
		-1: {Sent: 1, Failed: 1},
		-3: {Sent: 1, Failed: 0},
		-2: {}, -4: {}, -13: {},
	} {
		got := daily(s, day(offset).Format("2006-01-02"))
		if got.Sent != want.Sent || got.Failed != want.Failed {
			t.Errorf("day %+d = sent %d failed %d, want sent %d failed %d", offset, got.Sent, got.Failed, want.Sent, want.Failed)
		}
	}
	total := 0
	for _, d := range s.Daily {
		total += d.Sent
	}
	if total != 3 {
		t.Errorf("series total sent = %d, want 3 (the 20-day-old sends must be excluded)", total)
	}

	if len(s.NeedsAttention) != 2 {
		t.Fatalf("needs_attention = %+v, want 2 batches (C then A)", s.NeedsAttention)
	}
	first, second := s.NeedsAttention[0], s.NeedsAttention[1]
	if first.BatchID != cID || len(first.Reasons) != 1 || first.Reasons[0] != "high_invalid_rate" || first.Invalid != 3 || first.Total != 10 {
		t.Errorf("first attention item = %+v, want batch C flagged high_invalid_rate", first)
	}
	if second.BatchID != aID || len(second.Reasons) != 1 || second.Reasons[0] != "failed_jobs" || second.Failed != 2 {
		t.Errorf("second attention item = %+v, want batch A flagged failed_jobs (1 invalid of 9 is under the threshold)", second)
	}

	// days=1 shrinks the series to just today.
	one, err := st.GetStats(ctx, StatsOptions{Days: 1, Location: time.UTC})
	if err != nil || len(one.Daily) != 1 || one.Daily[0].Sent != 1 || one.Daily[0].Failed != 1 {
		t.Fatalf("days=1: %+v, %v", one.Daily, err)
	}
}

func TestGetStats_TimeZoneMovesDayBoundary(t *testing.T) {
	st := openTestStore(t)
	wipe(t, st)
	ctx := context.Background()

	dhaka, err := time.LoadLocation("Asia/Dhaka") // UTC+6, no daylight saving
	if err != nil {
		t.Skipf("tz database unavailable: %v", err)
	}
	now := time.Now().In(dhaka)
	dhakaToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, dhaka)

	// One email sent 1 minute after Dhaka midnight. That instant is 18:01 UTC on
	// the PREVIOUS calendar day, so the two zones must disagree about its day.
	_, ids := seedBatch(t, st, "sent")
	instant := dhakaToday.Add(time.Minute)
	setJob(t, st, ids[0], &instant, nil, 1)

	dh, err := st.GetStats(ctx, StatsOptions{Days: 3, Location: dhaka})
	if err != nil {
		t.Fatal(err)
	}
	if dh.Today.Sent != 1 {
		t.Errorf("Dhaka: today.sent = %d, want 1", dh.Today.Sent)
	}
	dhakaDate := dhakaToday.Format("2006-01-02")
	if got := daily(dh, dhakaDate); got.Sent != 1 {
		t.Errorf("Dhaka: bucket %s sent = %d, want 1 (%+v)", dhakaDate, got.Sent, dh.Daily)
	}

	utc, err := st.GetStats(ctx, StatsOptions{Days: 3, Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	utcDate := instant.UTC().Format("2006-01-02")
	if utcDate == dhakaDate {
		t.Fatalf("test setup wrong: both zones give %s", utcDate)
	}
	if got := daily(utc, utcDate); got.Sent != 1 {
		t.Errorf("UTC: bucket %s sent = %d, want 1 (%+v)", utcDate, got.Sent, utc.Daily)
	}
	if got := daily(utc, dhakaDate); got.Sent != 0 {
		t.Errorf("UTC: the Dhaka-today bucket must not contain it, got %d", got.Sent)
	}
}

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
)

var testSkipped = []string{"empty_email", "empty_name", "invalid_email", "duplicate", "bad_domain"}

// importRows saves rows as a new upload and deletes it on cleanup.
func importRows(t *testing.T, st *Store, rows []UploadRow) *Upload {
	t.Helper()
	up, err := st.ImportUpload(context.Background(), UploadMeta{Filename: "t.xlsx", SHA256: "abc", SkippedKinds: testSkipped},
		func(add func(UploadRow) error) error {
			for _, r := range rows {
				if err := add(r); err != nil {
					return err
				}
			}
			return nil
		})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	t.Cleanup(func() { st.db.Exec(`DELETE FROM uploads WHERE id = $1`, up.ID) })
	return up
}

func TestImportUpload_CountsAndChunks(t *testing.T) {
	st := openTestStore(t)

	// More rows than one COPY chunk, so several chunks are written.
	const n = copyChunkSize*2 + 7
	rows := make([]UploadRow, 0, n)
	for i := range n {
		kind := "ok"
		switch {
		case i%10 == 1:
			kind = "duplicate"
		case i%10 == 2:
			kind = "invalid_email"
		case i%10 == 3:
			kind = "unverified"
		}
		r := UploadRow{Row: i + 2, Name: fmt.Sprintf("P %d", i), Email: fmt.Sprintf("p%d@example.com", i), Kind: kind}
		if kind != "ok" {
			r.Reason = kind + " reason"
		}
		rows = append(rows, r)
	}
	up := importRows(t, st, rows)

	var dup, inv, unv int
	for _, r := range rows {
		switch r.Kind {
		case "duplicate":
			dup++
		case "invalid_email":
			inv++
		case "unverified":
			unv++
		}
	}
	if up.Status != "ready" || up.TotalRows != n || up.ProblemCount != dup+inv || up.UnverifiedCount != unv || up.ValidCount != n-dup-inv {
		t.Errorf("upload = %+v", up)
	}
	if up.ProblemCounts["duplicate"] != dup || up.ProblemCounts["invalid_email"] != inv || up.ProblemCounts["bad_domain"] != 0 {
		t.Errorf("problem counts = %v", up.ProblemCounts)
	}
	if !up.ExpiresAt.After(up.CreatedAt) {
		t.Errorf("expires_at %v should be after created_at %v", up.ExpiresAt, up.CreatedAt)
	}

	var stored int
	st.db.QueryRow(`SELECT count(*) FROM upload_rows WHERE upload_id = $1`, up.ID).Scan(&stored)
	if stored != n {
		t.Errorf("stored rows = %d, want %d", stored, n)
	}

	got, err := st.GetUpload(context.Background(), up.ID)
	if err != nil || got.ProblemCount != up.ProblemCount || got.ProblemCounts["duplicate"] != dup || got.Filename != "t.xlsx" {
		t.Errorf("GetUpload = %+v, %v", got, err)
	}
}

func TestImportUpload_FailureSavesNothing(t *testing.T) {
	st := openTestStore(t)
	var before int
	st.db.QueryRow(`SELECT count(*) FROM uploads`).Scan(&before)

	boom := errors.New("bad file")
	_, err := st.ImportUpload(context.Background(), UploadMeta{Filename: "bad.xlsx", SkippedKinds: testSkipped},
		func(add func(UploadRow) error) error {
			for i := range copyChunkSize + 1 { // one chunk already written before the failure
				if err := add(UploadRow{Row: i + 2, Name: "x", Email: fmt.Sprintf("x%d@e.com", i), Kind: "ok"}); err != nil {
					return err
				}
			}
			return boom
		})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the fill error", err)
	}
	var after int
	st.db.QueryRow(`SELECT count(*) FROM uploads`).Scan(&after)
	if after != before {
		t.Errorf("uploads %d -> %d: a failed import must leave nothing behind", before, after)
	}
}

func TestListUploadRows(t *testing.T) {
	st := openTestStore(t)
	up := importRows(t, st, []UploadRow{
		{Row: 2, Name: "Aisha", Email: "aisha@example.com", Kind: "ok"},
		{Row: 3, Name: "Ben", Email: "", Kind: "empty_email", Reason: "email is empty"},
		{Row: 5, Name: "Aisha R", Email: "aisha@example.com", Kind: "duplicate", Reason: "duplicate email"},
		{Row: 6, Name: "Carla", Email: "carla@yahoo.com", Kind: "unverified", Reason: "could not verify"},
		{Row: 7, Name: "Dev", Email: "dev@", Kind: "invalid_email", Reason: "invalid email"},
		{Row: 9, Name: "Eve", Email: "eve@example.com", Kind: "ok"},
	})
	ctx := context.Background()
	rowsOf := func(f UploadRowFilter) []int {
		t.Helper()
		rows, err := st.ListUploadRows(ctx, up.ID, f)
		if err != nil {
			t.Fatal(err)
		}
		var out []int
		for _, r := range rows {
			out = append(out, r.Row)
		}
		return out
	}

	if got := rowsOf(UploadRowFilter{Kinds: testSkipped, Limit: 10}); !slices.Equal(got, []int{3, 5, 7}) {
		t.Errorf("all problems = %v, want [3 5 7]", got)
	}
	if got := rowsOf(UploadRowFilter{Kinds: testSkipped, After: 3, Limit: 1}); !slices.Equal(got, []int{5}) {
		t.Errorf("after 3, limit 1 = %v, want [5]", got)
	}
	if got := rowsOf(UploadRowFilter{Kinds: []string{"unverified"}, Limit: 10}); !slices.Equal(got, []int{6}) {
		t.Errorf("unverified = %v, want [6]", got)
	}
	if got := rowsOf(UploadRowFilter{Kinds: testSkipped, Search: "AISHA", Limit: 10}); !slices.Equal(got, []int{5}) {
		t.Errorf("search = %v, want [5]", got)
	}
	if got := rowsOf(UploadRowFilter{Kinds: []string{"ok", "unverified"}, Limit: 2}); !slices.Equal(got, []int{2, 6}) {
		t.Errorf("sample = %v, want [2 6]", got)
	}

	rows, _ := st.ListUploadRows(ctx, up.ID, UploadRowFilter{Kinds: []string{"empty_email"}, Limit: 1})
	if len(rows) != 1 || rows[0].Reason != "email is empty" || rows[0].Name != "Ben" {
		t.Errorf("row fields = %+v", rows)
	}
}

func TestExpiredUploads(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	up := importRows(t, st, []UploadRow{{Row: 2, Name: "A", Email: "a@example.com", Kind: "ok"}})

	if _, err := st.db.Exec(`UPDATE uploads SET expires_at = NOW() - INTERVAL '1 minute' WHERE id = $1`, up.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetUpload(ctx, up.ID); !errors.Is(err, ErrUploadNotFound) {
		t.Errorf("expired upload: err = %v, want ErrUploadNotFound", err)
	}
	if _, err := st.GetUpload(ctx, 0); !errors.Is(err, ErrUploadNotFound) {
		t.Errorf("unknown upload: err = %v, want ErrUploadNotFound", err)
	}

	n, err := st.DeleteExpiredUploads(ctx)
	if err != nil || n < 1 {
		t.Fatalf("deleted = %d, %v; want at least 1", n, err)
	}
	var rows int
	st.db.QueryRow(`SELECT count(*) FROM upload_rows WHERE upload_id = $1`, up.ID).Scan(&rows)
	if rows != 0 {
		t.Errorf("rows of a deleted upload = %d, want 0 (cascade)", rows)
	}
}

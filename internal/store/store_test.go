package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
)

// openTestStore connects to TEST_DATABASE_URL, or skips the test when unset.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return New(db)
}

func TestListBatches(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	items, total, err := st.ListBatches(ctx, BatchListFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListBatches: %v", err)
	}
	if items == nil {
		t.Fatal("items must be non-nil so it encodes as []")
	}
	if len(items) > 5 || len(items) > total {
		t.Fatalf("got %d items for limit 5, total %d", len(items), total)
	}
	for i := 1; i < len(items); i++ {
		if items[i-1].ID <= items[i].ID {
			t.Fatalf("not newest-first: %d then %d", items[i-1].ID, items[i].ID)
		}
	}

	// A status nothing has must yield an empty page, not an error.
	none, n, err := st.ListBatches(ctx, BatchListFilter{Status: "no-such-status", Limit: 5})
	if err != nil || len(none) != 0 || n != 0 {
		t.Fatalf("unknown status: items=%d total=%d err=%v", len(none), n, err)
	}
}

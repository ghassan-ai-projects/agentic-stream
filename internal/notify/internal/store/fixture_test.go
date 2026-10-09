package store_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var at = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func openStore(t *testing.T) (store.Store, *storage.DB) {
	t.Helper()
	db := storagetest.OpenTemp(t)
	return store.New(db), db
}

func sha(seed int64) []byte {
	digest := make([]byte, 32)
	digest[0] = byte(seed & 0xff)
	return digest
}

func row(eventID string, cursor int64, created time.Time) store.Notification {
	return store.Notification{TenantID: "t", EventID: eventID, EventType: "x", Cursor: cursor, EventJSON: []byte("{}"), EventSHA: sha(cursor), CreatedAt: created}
}

func insertRows(t *testing.T, tx *store.Tx, ids ...string) {
	t.Helper()
	for i, id := range ids {
		if inserted, err := tx.InsertNotification(t.Context(), row(id, int64(i+1), at)); err != nil || !inserted {
			t.Fatalf("insert %s: inserted=%v err=%v", id, inserted, err)
		}
	}
}

func countRows(t *testing.T, db *storage.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), query).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

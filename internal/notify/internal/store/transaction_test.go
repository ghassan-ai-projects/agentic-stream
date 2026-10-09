package store_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
)

func TestAStoreIsConfiguredOnlyWithItsDatabase(t *testing.T) {
	t.Parallel()
	if store.New(nil).Configured() {
		t.Fatal("nil database reported as configured")
	}
	persistence, _ := openStore(t)
	if !persistence.Configured() {
		t.Fatal("database not reported as configured")
	}
}

func TestAJoinedTransactionStaysUnderTheCallersOwnership(t *testing.T) {
	t.Parallel()
	persistence, db := openStore(t)
	rollback := errors.New("caller rolls back")
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		cursor, err := store.Join(tx).AllocateCursor(t.Context(), "t")
		if err != nil || cursor != 1 {
			t.Fatalf("cursor=%d err=%v", cursor, err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("err=%v", err)
	}
	if bounds, err := persistence.Autocommit().TenantBounds(t.Context(), "t"); err != nil || bounds.HasNextCursor {
		t.Fatalf("rolled-back cursor persisted: %+v err=%v", bounds, err)
	}
}

package store_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
)

func TestAuditRowsRecordRefusals(t *testing.T) {
	t.Parallel()
	persistence, db := openStore(t)
	audit := store.Audit{ID: "a1", TenantID: "t", Action: "cursor_expired", Requested: 3, Oldest: 5, Details: []byte(`{}`), At: at}
	if err := persistence.Autocommit().RecordAudit(t.Context(), audit); err != nil {
		t.Fatal(err)
	}
	var action string
	var requested, oldest int64
	if err := db.QueryRowContext(t.Context(), "SELECT action, requested_cursor, oldest_cursor FROM notification_audits WHERE audit_id = 'a1'").Scan(&action, &requested, &oldest); err != nil {
		t.Fatal(err)
	}
	if action != "cursor_expired" || requested != 3 || oldest != 5 {
		t.Fatalf("audit = %s %d %d", action, requested, oldest)
	}
}

func TestEveryOperationNamesItselfWhenTheDatabaseFails(t *testing.T) {
	t.Parallel()
	operations := []struct {
		name string
		run  func(context.Context, *store.Tx) error
		want string
	}{
		{"find notification", func(ctx context.Context, tx *store.Tx) error {
			_, _, err := tx.FindNotification(ctx, "t", "a")
			return err
		}, "check duplicate notification"},
		{"find tombstone", func(ctx context.Context, tx *store.Tx) error {
			_, _, err := tx.FindTombstone(ctx, "t", "a")
			return err
		}, "check notification tombstone"},
		{"allocate cursor", func(ctx context.Context, tx *store.Tx) error { _, err := tx.AllocateCursor(ctx, "t"); return err }, "allocate notification cursor"},
		{"insert notification", func(ctx context.Context, tx *store.Tx) error {
			_, err := tx.InsertNotification(ctx, row("a", 1, at))
			return err
		}, "append notification"},
		{"stored digest", func(ctx context.Context, tx *store.Tx) error { _, err := tx.StoredSHA(ctx, "t", "a"); return err }, "read duplicate notification"},
		{"release cursor", func(ctx context.Context, tx *store.Tx) error { return tx.ReleaseCursor(ctx, "t", 1) }, "rollback duplicate cursor"},
		{"notification cursor", func(ctx context.Context, tx *store.Tx) error {
			_, err := tx.NotificationCursor(ctx, "t", "a")
			return err
		}, "read notification cursor"},
		{"record audit", func(ctx context.Context, tx *store.Tx) error {
			return tx.RecordAudit(ctx, store.Audit{ID: "a", At: at})
		}, "write notification audit"},
		{"count poison attempt", func(ctx context.Context, tx *store.Tx) error {
			_, err := tx.CountPoisonAttempt(ctx, "t", 1, at)
			return err
		}, "record notification poison attempt"},
		{"clear poison attempts", func(ctx context.Context, tx *store.Tx) error { return tx.ClearPoisonAttempts(ctx, "t", 1) }, "clear notification poison attempts"},
		{"tenant bounds", func(ctx context.Context, tx *store.Tx) error { _, err := tx.TenantBounds(ctx, "t"); return err }, "find oldest notification"},
		{"read rows", func(ctx context.Context, tx *store.Tx) error { _, err := tx.ReadRows(ctx, "t", 0, 1); return err }, "read notifications"},
		{"retire notifications", func(ctx context.Context, tx *store.Tx) error { return tx.RetireNotifications(ctx, at, at) }, "tombstone notifications"},
		{"delete retired notifications", func(ctx context.Context, tx *store.Tx) error {
			_, err := tx.DeleteRetiredNotifications(ctx, at)
			return err
		}, "prune notifications"},
		{"delete expired tombstones", func(ctx context.Context, tx *store.Tx) error { return tx.DeleteExpiredTombstones(ctx, at) }, "prune notification tombstones"},
		{"count retirable notifications", func(ctx context.Context, tx *store.Tx) error {
			_, err := tx.CountNotificationsBefore(ctx, at)
			return err
		}, "count retirable notifications"},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()
			persistence, db := openStore(t)
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if err := operation.run(t.Context(), persistence.Autocommit()); err == nil || !strings.Contains(err.Error(), operation.want) {
				t.Fatalf("err = %v, want it to name %q", err, operation.want)
			}
		})
	}
}

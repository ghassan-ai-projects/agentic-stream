package approvalledger_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
)

func TestWithdrawalReadsClockAfterEachOrderedMutationAndRollsBackOnPublishFailure(t *testing.T) {
	db := approvalDB(t)
	ctx := t.Context()
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		for _, id := range []string{"old", "denied"} {
			if err := approvalledger.Request(ctx, tx, id, id, "now", "later", []byte(`{}`), "nonce-"+id); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `CREATE TRIGGER fail_second_notification BEFORE INSERT ON notifications
			WHEN NEW.event_id = 'approval.withdrawn:old' BEGIN SELECT RAISE(ABORT, 'publication failed'); END`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	reads := 0
	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		clk := withdrawalClock{Clock: clock.NewVirtual(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)), beforeRead: func() {
			id := []string{"denied", "old"}[reads]
			var state string
			if err := tx.QueryRowContext(ctx, "SELECT status FROM approvals WHERE approval_id=?", id).Scan(&state); err != nil {
				t.Fatal(err)
			}
			if state != "denied" {
				t.Fatalf("clock read preceded withdrawal of %s: %s", id, state)
			}
			reads++
		}}
		return approvalledger.WithdrawSuperseded(ctx, tx, "situation", 2, "withdrawn", withdrawalPublisher("tenant", clk))
	})
	if err == nil || !strings.Contains(err.Error(), "append superseded approval notification:") || reads != 2 {
		t.Fatalf("ordered publication failure: err=%v clock reads=%d", err, reads)
	}
	for _, id := range []string{"denied", "old"} {
		var state string
		if err := db.QueryRowContext(ctx, "SELECT status FROM approvals WHERE approval_id=?", id).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state != "pending" {
			t.Fatalf("withdrawal escaped failed publication: %s=%s", id, state)
		}
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("notification escaped failed publication: count=%d", count)
	}
}

type withdrawalClock struct {
	clock.Clock
	beforeRead func()
}

func (c withdrawalClock) Now() time.Time {
	c.beforeRead()
	return c.Clock.Now()
}

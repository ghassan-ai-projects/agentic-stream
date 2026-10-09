package approvalledger_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestSupersededWithdrawalAndNotificationShareTheCallersTransaction(t *testing.T) {
	t.Parallel()
	db := approvalDB(t)
	clk := sources.NewVirtual(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC))
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		if err := requestApproval(ctx, tx, "old", "old", "later"); err != nil {
			return err
		}
		return requestApproval(ctx, tx, "current", "current", "later")
	})
	withdraw := func(tx *sql.Tx) error {
		return approvalledger.WithdrawSuperseded(t.Context(), tx, "situation", 2, "withdrawn", withdrawalPublisher("tenant", clk))
	}
	rollback := errors.New("downstream participant failed")
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if err := withdraw(tx); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("withdrawal rollback: %v", err)
	}
	if state, notes := statusOf(t, db, "old"), countNotifications(t, db); state != "pending" || notes != 0 {
		t.Fatalf("withdrawal escaped rollback: status=%s notifications=%d", state, notes)
	}
	inTx(t, db, func(_ context.Context, tx *sql.Tx) error { return withdraw(tx) })
	if old, current, notes := statusOf(t, db, "old"), statusOf(t, db, "current"), countNotifications(t, db); old != "denied" || current != "pending" || notes != 1 {
		t.Fatalf("after withdrawal: old=%s current=%s notifications=%d, want denied, pending and one notification", old, current, notes)
	}
}

func TestWithdrawingSupersededApprovalsRequiresAPublisher(t *testing.T) {
	t.Parallel()
	db := approvalDB(t)
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return approvalledger.WithdrawSuperseded(t.Context(), tx, "situation", 2, "withdrawn", nil)
	})
	if !errors.Is(err, approvalledger.ErrPublisherRequired) {
		t.Fatalf("silent withdrawal accepted: %v", err)
	}
}

type withdrawalClock struct {
	sources.Clock
	beforeRead func()
}

func (c withdrawalClock) Now() time.Time {
	c.beforeRead()
	return c.Clock.Now()
}

func TestWithdrawalReadsClockAfterEachOrderedMutationAndRollsBackOnPublishFailure(t *testing.T) {
	t.Parallel()
	db := approvalDB(t)
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		for _, id := range []string{"old", "denied"} {
			if err := requestApproval(ctx, tx, id, id, "later"); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `CREATE TRIGGER fail_second_notification BEFORE INSERT ON notifications
			WHEN NEW.event_id = 'approval.withdrawn:old' BEGIN SELECT RAISE(ABORT, 'publication failed'); END`)
		return err
	})
	reads := 0
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		clk := withdrawalClock{Clock: sources.NewVirtual(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)), beforeRead: func() {
			id := []string{"denied", "old"}[reads]
			var state string
			if err := tx.QueryRowContext(t.Context(), "SELECT status FROM approvals WHERE approval_id=?", id).Scan(&state); err != nil {
				t.Error(err)
			}
			if state != "denied" {
				t.Errorf("clock read preceded withdrawal of %s: %s", id, state)
			}
			reads++
		}}
		return approvalledger.WithdrawSuperseded(t.Context(), tx, "situation", 2, "withdrawn", withdrawalPublisher("tenant", clk))
	})
	if err == nil || !strings.Contains(err.Error(), "append superseded approval notification:") || reads != 2 {
		t.Fatalf("ordered publication failure: err=%v clock reads=%d", err, reads)
	}
	for _, id := range []string{"denied", "old"} {
		if state := statusOf(t, db, id); state != "pending" {
			t.Fatalf("withdrawal escaped failed publication: %s=%s", id, state)
		}
	}
	if notes := countNotifications(t, db); notes != 0 {
		t.Fatalf("notification escaped failed publication: count=%d", notes)
	}
}

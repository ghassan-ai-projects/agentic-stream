package costcontrol_test

import (
	"database/sql"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestReservationRollsBackGlobalAccountingOnTenantRejection(t *testing.T) {
	t.Parallel()
	db := newCostDB(t)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return costcontrol.SetLimit(t.Context(), tx, "tenant:tenant", "tenant", 3, false, "now")
	}); err != nil {
		t.Fatal(err)
	}
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return (costcontrol.Controller{}).Reserve(t.Context(), tx, "episode", "tenant", 4, "now")
	})
	if !errors.Is(err, costcontrol.ErrReservationRejected) || !strings.Contains(err.Error(), "tenant:tenant") {
		t.Fatalf("tenant rejection = %v", err)
	}
	assertCostTotals(t, db, 0, 0, 0)
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM cost_reservations").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected reservation persisted: count=%d err=%v", count, err)
	}
}

func TestGlobalCostRejectionPrecedesTenantRejection(t *testing.T) {
	t.Parallel()
	db := newCostDB(t)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if err := costcontrol.SetLimit(t.Context(), tx, "global", "", 1, true, "now"); err != nil {
			return err
		}
		return costcontrol.SetLimit(t.Context(), tx, "tenant:tenant", "tenant", 1, true, "now")
	}); err != nil {
		t.Fatal(err)
	}
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return (costcontrol.Controller{}).Reserve(t.Context(), tx, "episode", "tenant", 1, "now")
	})
	if !errors.Is(err, costcontrol.ErrReservationRejected) || !strings.HasSuffix(err.Error(), ": global") {
		t.Fatalf("scope precedence = %v", err)
	}
	assertCostTotals(t, db, 0, 0, 1)
}

func TestSettlementRollsBackAllAccountingOnTenantWriteFailure(t *testing.T) {
	t.Parallel()
	db := newCostDB(t)
	controller := costcontrol.Controller{}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if err := costcontrol.SetLimit(t.Context(), tx, "tenant:tenant", "tenant", 10, false, "now"); err != nil {
			return err
		}
		return controller.Reserve(t.Context(), tx, "episode", "tenant", 4, "now")
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER reject_tenant_settlement
		BEFORE UPDATE ON cost_limits WHEN NEW.scope_key = 'tenant:tenant' AND NEW.spent_micro > 0
		BEGIN SELECT RAISE(ABORT, 'injected tenant failure'); END`); err != nil {
		t.Fatal(err)
	}
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return controller.Settle(t.Context(), tx, "episode", 2, "later") })
	if err == nil || !strings.Contains(err.Error(), "settle tenant:tenant cost") {
		t.Fatalf("settlement failure = %v", err)
	}
	assertCostTotals(t, db, 4, 0, 0)
	var status string
	var actual int
	if err := db.QueryRowContext(t.Context(), "SELECT status, actual_micro FROM cost_reservations WHERE episode_id = 'episode'").Scan(&status, &actual); err != nil || status != "reserved" || actual != 0 {
		t.Fatalf("failed settlement changed reservation: status=%q actual=%d err=%v", status, actual, err)
	}
}

func TestRepeatedSettlementDoesNotSpendTwiceOrChangeKillSwitch(t *testing.T) {
	t.Parallel()
	db := newCostDB(t)
	controller := costcontrol.Controller{}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if err := costcontrol.SetLimit(t.Context(), tx, "global", "", 5, false, "now"); err != nil {
			return err
		}
		return controller.Reserve(t.Context(), tx, "episode", "tenant", 4, "now")
	}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return controller.Settle(t.Context(), tx, "episode", 5, "later") }); err != nil {
			t.Fatal(err)
		}
		assertCostTotals(t, db, 0, 5, 1)
	}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return controller.Settle(t.Context(), tx, "episode", 6, "later") }); err == nil || !strings.Contains(err.Error(), "already settled with a different value") {
		t.Fatalf("conflicting settlement = %v", err)
	}
	assertCostTotals(t, db, 0, 5, 1)
}

func TestCostRangeValidationPrecedesTransactionAccess(t *testing.T) {
	t.Parallel()
	controller := costcontrol.Controller{}
	if err := controller.Reserve(t.Context(), nil, "episode", "tenant", math.MaxUint64, "now"); err == nil || err.Error() != "invalid cost reservation" {
		t.Fatalf("overflow reservation = %v", err)
	}
	if err := controller.Settle(t.Context(), nil, "episode", math.MaxUint64, "now"); err == nil || err.Error() != "invalid cost settlement" {
		t.Fatalf("overflow settlement = %v", err)
	}
	if err := costcontrol.SetLimit(t.Context(), nil, "global", "", math.MaxUint64, false, "now"); err == nil || err.Error() != "invalid cost limit" {
		t.Fatalf("overflow limit = %v", err)
	}
}

func newCostDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "cost.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func assertCostTotals(t *testing.T, db *storage.DB, reserved, spent, killed int) {
	t.Helper()
	var gotReserved, gotSpent, gotKilled int
	if err := db.QueryRowContext(t.Context(), "SELECT reserved_micro, spent_micro, kill_switch FROM cost_limits WHERE scope_key = 'global'").Scan(&gotReserved, &gotSpent, &gotKilled); err != nil {
		t.Fatal(err)
	}
	if gotReserved != reserved || gotSpent != spent || gotKilled != killed {
		t.Fatalf("global accounting = (%d,%d,%d), want (%d,%d,%d)", gotReserved, gotSpent, gotKilled, reserved, spent, killed)
	}
}

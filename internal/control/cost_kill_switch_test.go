package control_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/controltest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const costNow = "2026-08-12T12:00:00Z"

func reserveAs(t *testing.T, db *storage.DB, episodeID, tenantID string, amount uint64) error {
	t.Helper()
	return db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return (control.CostLedger{}).Reserve(t.Context(), tx, episodeID, tenantID, amount, costNow)
	})
}

func setLimit(t *testing.T, db *storage.DB, scope, tenantID string, maxMicro uint64, kill bool) {
	t.Helper()
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return controltest.SetCostLimit(t.Context(), tx, scope, tenantID, maxMicro, kill, costNow)
	}); err != nil {
		t.Fatalf("set %s limit: %v", scope, err)
	}
}

func TestTheKillSwitchRefusesEveryNewReservationButNotTheSettlementOfHeldOnes(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	setLimit(t, db, "global", "", 10, false)
	if err := reserveAs(t, db, "episode-1", "tenant-1", 7); err != nil {
		t.Fatalf("reserve under the ceiling: %v", err)
	}
	setLimit(t, db, "global", "", 10, true)
	if err := reserveAs(t, db, "episode-2", "tenant-1", 1); !errors.Is(err, control.ErrCostReservationRejected) {
		t.Fatalf("reservation under a tripped kill switch = %v, want ErrCostReservationRejected", err)
	}
	settle := func(tx *sql.Tx) error { return (control.CostLedger{}).Settle(t.Context(), tx, "episode-1", 6, costNow) }
	if err := db.WithTx(t.Context(), settle); err != nil {
		t.Fatalf("settling a held reservation under the kill switch: %v", err)
	}
	assertCostTotals(t, db, 0, 6, 1)
}

func TestACeilingRefusesAReservationThatWouldExceedItAndAZeroEstimateUnderAnyCeiling(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	setLimit(t, db, "tenant:tenant-1", "tenant-1", 10, false)
	if err := reserveAs(t, db, "episode-1", "tenant-1", 6); err != nil {
		t.Fatalf("reserve under the ceiling: %v", err)
	}
	for name, attempt := range map[string]error{
		"a reservation beyond the remaining ceiling": reserveAs(t, db, "episode-2", "tenant-1", 5),
		"an unestimated reservation":                 reserveAs(t, db, "episode-3", "tenant-1", 0),
	} {
		if !errors.Is(attempt, control.ErrCostReservationRejected) {
			t.Errorf("%s = %v, want ErrCostReservationRejected", name, attempt)
		}
	}
}

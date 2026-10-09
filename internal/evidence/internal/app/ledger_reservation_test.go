package app

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
)

func TestLedgerReservesOnceAndReplaysTheCompletedResult(t *testing.T) {
	t.Parallel()
	ledger, _ := newLedger(t)
	call := ledgerTestCall()
	first := reserveTestCall(t, ledger)
	if first.Completed != nil || first.Status != "running" || !first.Created {
		t.Fatalf("first reservation = %+v", first)
	}
	result := QueryResult{JSON: []byte(`{"rows":[{"value":42}]}`), RowCount: 1}
	if err := ledger.Complete(t.Context(), first, result); err != nil {
		t.Fatalf("complete: %v", err)
	}

	second, err := ledger.Reserve(t.Context(), call, "token-1", "epoch-1")
	if err != nil {
		t.Fatalf("replay reserve: %v", err)
	}
	if second.Completed == nil || string(second.Completed.JSON) != string(result.JSON) || second.Completed.RowCount != 1 || second.Created {
		t.Fatalf("replayed reservation = %+v", second)
	}
}

func TestLedgerRefusesToReuseACallIdentityForAnotherRequestOrToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Call)
		token  string
		epoch  string
		want   string
	}{
		{"another entity argument", func(c *Call) { c.Arguments = EvidenceGetArguments{EntityID: "other"} }, "token-1", "epoch-1", "different request"},
		{"a wider byte budget", func(c *Call) { c.MaxBytes++ }, "token-1", "epoch-1", "different request"},
		{"a later range end", func(c *Call) { c.Until = c.Until.Add(1) }, "token-1", "epoch-1", "different request"},
		{"another capability token", func(*Call) {}, "token-2", "epoch-1", "token identity mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ledger, _ := newLedger(t)
			reserveTestCall(t, ledger)
			call := ledgerTestCall()
			test.mutate(&call)
			_, err := ledger.Reserve(t.Context(), call, test.token, test.epoch)
			requireContains(t, err, test.want)
		})
	}
}

func TestLedgerRefusesToReserveWithoutCompleteConfiguration(t *testing.T) {
	t.Parallel()
	db := openLedgerDB(t)
	tests := []struct {
		name         string
		ledger       *Ledger
		token, epoch string
	}{
		{"nil ledger", nil, "token", "epoch-1"},
		{"no store", &Ledger{LeaseOwner: "owner", RuntimeEpoch: "epoch-1"}, "token", "epoch-1"},
		{"no lease owner", &Ledger{Store: ledgerOn(db, allowOwner, "epoch-1").Store, RuntimeEpoch: "epoch-1"}, "token", "epoch-1"},
		{"no ledger epoch", &Ledger{Store: ledgerOn(db, allowOwner, "epoch-1").Store, LeaseOwner: "owner"}, "token", "epoch-1"},
		{"no call epoch", ledgerOn(db, allowOwner, "epoch-1"), "token", ""},
		{"another call epoch", ledgerOn(db, allowOwner, "epoch-1"), "token", "epoch-2"},
		{"no token identity", ledgerOn(db, allowOwner, "epoch-1"), "", "epoch-1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := test.ledger.Reserve(t.Context(), ledgerTestCall(), test.token, test.epoch)
			requireContains(t, err, "ledger is not configured")
		})
	}
}

func TestLedgerConcurrentReservationHasOneWinner(t *testing.T) {
	t.Parallel()
	ledger, _ := newLedger(t)
	var winners atomic.Int32
	results := make(chan error, 2)
	for range 2 {
		go func() {
			reservation, err := ledger.Reserve(context.Background(), ledgerTestCall(), "token", "epoch-1")
			if err == nil && reservation.Created {
				winners.Add(1)
			}
			results <- err
		}()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Errorf("reserve: %v", err)
		}
	}
	if winners.Load() != 1 {
		t.Fatalf("reservation winners = %d, want 1", winners.Load())
	}
}

func TestLedgerReservesOnlyLiveAttemptsOfTheCurrentEpisode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		statement string
		mutate    func(*Call)
		want      string
	}{
		{"superseded episode", "UPDATE episodes SET lifecycle_status = 'superseded'", nil, "attempt is stale"},
		{"newer fence on the episode", "UPDATE episodes SET current_fence = 2", nil, "attempt is stale"},
		{"terminal attempt", "UPDATE episode_attempts SET status = 'failed'", nil, "attempt is terminal"},
		{"unknown episode", "SELECT 1", func(c *Call) { c.EpisodeID = "episode-9" }, "episode is unknown"},
		{"another tenant", "SELECT 1", func(c *Call) { c.TenantID = "tenant-2" }, "episode is unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ledger, db := newLedger(t)
			execSQL(t, db, test.statement)
			call := ledgerTestCall()
			if test.mutate != nil {
				test.mutate(&call)
			}
			_, err := ledger.Reserve(t.Context(), call, "token-1", "epoch-1")
			requireContains(t, err, test.want)
			if ledgerRows(t, db) != 0 {
				t.Fatal("a refused reservation left a ledger row")
			}
		})
	}
}

func TestLedgerOwnerLossRefusesReservationWithoutLeavingARow(t *testing.T) {
	t.Parallel()
	db := openLedgerDB(t)
	lost := errors.New("owner lost")
	ledger := ledgerOn(db, func(context.Context, *sql.Tx, string) error { return lost }, "epoch-1")
	_, err := ledger.Reserve(t.Context(), ledgerTestCall(), "token-1", "epoch-1")
	if !errors.Is(err, lost) {
		t.Fatalf("reserve error = %v, want the owner loss", err)
	}
	if ledgerRows(t, db) != 0 {
		t.Fatal("a refused reservation left a ledger row")
	}
}

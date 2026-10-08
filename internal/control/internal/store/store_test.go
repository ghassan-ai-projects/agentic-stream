package store_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var instant = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

var now = kernel.FormatTime(instant)

func openStore(t *testing.T) (store.Store, *storage.DB) {
	t.Helper()
	db := storagetest.OpenTemp(t)

	return store.New(db), db
}

func TestStoreReportsConfigurationAndOpenness(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	if store.New(nil).Configured() || !persistence.Configured() {
		t.Fatal("configuration report changed")
	}
	if store.Join(nil).Open() || !persistence.Autocommit().Open() {
		t.Fatal("openness report changed")
	}
}

func TestOwnerLeaseClaimRenewReleaseAndHold(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	later := instant.Add(time.Minute)
	if err := tx.ClaimLease(t.Context(), "e1", "i1", instant, later); err != nil {
		t.Fatal(err)
	}
	if epoch, instance, err := tx.RecordedOwner(t.Context()); err != nil || epoch != "e1" || instance != "i1" {
		t.Fatalf("recorded = %s %s %v", epoch, instance, err)
	}
	if held, err := tx.HoldsLease(t.Context(), "e1", "i1", instant); err != nil || !held {
		t.Fatalf("held=%v err=%v", held, err)
	}
	if held, _ := tx.HoldsLease(t.Context(), "e2", "i1", instant); held {
		t.Fatal("other epoch holds the lease")
	}
	if rows, err := tx.RenewLease(t.Context(), "e1", "i1", instant, later); err != nil || rows != 1 {
		t.Fatalf("renew rows=%d err=%v", rows, err)
	}
	if rows, _ := tx.RenewLease(t.Context(), "e2", "i1", instant, later); rows != 0 {
		t.Fatalf("other epoch renewed: %d", rows)
	}
	if rows, err := tx.ReleaseLease(t.Context(), "e1", "i1", instant); err != nil || rows != 1 {
		t.Fatalf("release rows=%d err=%v", rows, err)
	}
	if held, _ := tx.HoldsLease(t.Context(), "e1", "i1", instant); held {
		t.Fatal("released lease still held")
	}
}

func TestRecoverHandsTheClaimingTransactionToTheCaller(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	boom := errors.New("recovery failed")
	err := persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		return tx.Recover(func(raw *sql.Tx, _ time.Time) error {
			if raw == nil {
				t.Error("recovery received no transaction")
			}
			return boom
		}, time.Now())
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestEpochStateRecordIsTerminalOnceKilled(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	if _, found, err := tx.EpochState(t.Context(), "e", "epoch control"); err != nil || found {
		t.Fatalf("uncontrolled epoch found=%v err=%v", found, err)
	}
	for _, state := range []string{"draining", "killed", "draining"} {
		if err := tx.RecordEpochState(t.Context(), "e", state, instant); err != nil {
			t.Fatal(err)
		}
	}
	if state, found, err := tx.EpochState(t.Context(), "e", "epoch control"); err != nil || !found || state != "killed" {
		t.Fatalf("state=%q found=%v err=%v", state, found, err)
	}
}

func TestCostLimitsReservationsAndSettlement(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	ctx, scope := t.Context(), "tenant:t"
	if exists, _ := tx.LimitExists(ctx, scope); exists {
		t.Fatal("limit exists before it is written")
	}
	if rows, err := tx.WriteLimit(ctx, scope, "t", 10, false, now); err != nil || rows != 1 {
		t.Fatalf("write rows=%d err=%v", rows, err)
	}
	if maxMicro, kill, err := tx.ReadLimit(ctx, scope); err != nil || maxMicro != 10 || kill {
		t.Fatalf("limit = %d %v %v", maxMicro, kill, err)
	}
	if rows, _ := tx.ReserveAvailable(ctx, scope, 11, now); rows != 0 {
		t.Fatalf("over-ceiling reservation admitted: %d", rows)
	}
	if rows, err := tx.ReserveAvailable(ctx, scope, 6, now); err != nil || rows != 1 {
		t.Fatalf("reserve rows=%d err=%v", rows, err)
	}
	if rows, err := tx.SettleLimit(ctx, scope, 6, 10, now); err != nil || rows != 1 {
		t.Fatalf("settle rows=%d err=%v", rows, err)
	}
	if _, kill, _ := tx.ReadLimit(ctx, scope); !kill {
		t.Fatal("spend reaching the ceiling did not trip the kill switch")
	}
	if _, found, _ := tx.LoadReservation(ctx, "ep"); found {
		t.Fatal("reservation found before it is inserted")
	}
}

func TestReservationInsertLoadAndSettlementRecord(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	ctx := t.Context()
	if err := tx.InsertReservation(ctx, "ep", "t", 7, now); err != nil {
		t.Fatal(err)
	}
	if err := tx.InsertReservation(ctx, "ep", "t", 7, now); err == nil {
		t.Fatal("duplicate reservation accepted")
	}
	reservation, found, err := tx.LoadReservation(ctx, "ep")
	if err != nil || !found || reservation.Reserved != 7 || reservation.TenantID != "t" || reservation.Status != "reserved" {
		t.Fatalf("reservation = %+v found=%v err=%v", reservation, found, err)
	}
	if err := tx.RecordSettlement(ctx, "ep", 5, now); err != nil {
		t.Fatal(err)
	}
	if settled, _, _ := tx.LoadReservation(ctx, "ep"); settled.Status != "settled" || settled.Actual != 5 {
		t.Fatalf("settled = %+v", settled)
	}
}

func TestCeilingsOfReadsBothScopes(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	if _, err := tx.WriteLimit(t.Context(), "tenant:t", "t", 9, true, now); err != nil {
		t.Fatal(err)
	}
	ceilings, err := tx.CeilingsOf(t.Context(), "global", "tenant:t")
	if err != nil || len(ceilings) != 2 {
		t.Fatalf("ceilings = %+v err=%v", ceilings, err)
	}
	found := false
	for _, ceiling := range ceilings {
		found = found || (ceiling.Scope == "tenant:t" && ceiling.MaxMicro == 9 && ceiling.KillSwitch)
	}
	if !found {
		t.Fatalf("tenant ceiling missing: %+v", ceilings)
	}
}

func TestUnstartedReservedEpisodesIsEmptyWithoutEpisodes(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	episodes, err := persistence.Autocommit().UnstartedReservedEpisodes(t.Context(), "e")
	if err != nil || len(episodes) != 0 {
		t.Fatalf("episodes = %v err=%v", episodes, err)
	}
}

func TestHoldsLeaseJudgesEpochInstanceAndInstantTogether(t *testing.T) {
	t.Parallel()
	until := instant.Add(500 * time.Millisecond)
	for name, tc := range map[string]struct {
		epoch, instance string
		at              time.Time
		want            bool
	}{
		"owner before the lease ends":               {"e1", "i1", until.Add(-time.Nanosecond), true},
		"owner at a whole second before a fraction": {"e1", "i1", instant, true},
		"owner exactly at the lease end":            {"e1", "i1", until, false},
		"owner after the lease":                     {"e1", "i1", instant.Add(time.Second), false},
		"same epoch, another instance":              {"e1", "i2", instant, false},
		"another epoch, same instance":              {"e2", "i1", instant, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			persistence, _ := openStore(t)
			tx := persistence.Autocommit()
			if err := tx.ClaimLease(t.Context(), "e1", "i1", instant, until); err != nil {
				t.Fatal(err)
			}
			held, err := tx.HoldsLease(t.Context(), tc.epoch, tc.instance, tc.at)
			if err != nil || held != tc.want {
				t.Fatalf("held=%v err=%v, want %v", held, err, tc.want)
			}
		})
	}
}

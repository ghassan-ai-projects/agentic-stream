package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var base = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func openStore(t *testing.T) store.Store {
	t.Helper()
	db, err := storagetest.Open(t.Context(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store.New(db)
}

func owner(persistence store.Store, instance string, now func() time.Time) *Owner {
	return &Owner{Store: persistence, Instance: instance, Lease: time.Minute, Now: now}
}

func TestUnconfiguredControlRefusesEveryOperation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if err := Claim(ctx, nil, "e"); !errors.Is(err, domain.ErrOwnerNotConfigured) {
		t.Fatalf("nil owner claim = %v", err)
	}
	if err := Renew(ctx, &Owner{}, "e"); !errors.Is(err, domain.ErrOwnerNotConfigured) {
		t.Fatalf("no-database renew = %v", err)
	}
	if err := Assert(ctx, &Owner{}, store.Join(nil), "e"); !errors.Is(err, domain.ErrOwnerNotConfigured) {
		t.Fatalf("closed-transaction assert = %v", err)
	}
	if err := Kill(ctx, nil, "e"); !errors.Is(err, domain.ErrEpochControlNotConfigured) {
		t.Fatalf("nil epochs kill = %v", err)
	}
	if err := AssertAdmission(ctx, &Epochs{}, "e"); !errors.Is(err, domain.ErrEpochControlNotConfigured) {
		t.Fatalf("admission = %v", err)
	}
	if err := AuthorizeDispatch(ctx, store.New(nil)); !errors.Is(err, domain.ErrDispatchGateNotConfigured) {
		t.Fatalf("dispatch = %v", err)
	}
}

func TestAnotherInstanceCannotClaimAnUnexpiredLease(t *testing.T) {
	t.Parallel()
	persistence := openStore(t)
	clock := func() time.Time { return base }
	if err := Claim(t.Context(), owner(persistence, "a", clock), "e1"); err != nil {
		t.Fatal(err)
	}
	if err := Claim(t.Context(), owner(persistence, "b", clock), "e2"); !errors.Is(err, domain.ErrRuntimeOwnerBusy) {
		t.Fatalf("second epoch claim = %v", err)
	}
	later := func() time.Time { return base.Add(2 * time.Minute) }
	if err := Renew(t.Context(), owner(persistence, "a", later), "e1"); !errors.Is(err, domain.ErrRuntimeOwnerBusy) {
		t.Fatalf("expired renew = %v", err)
	}
	if err := Claim(t.Context(), owner(persistence, "b", later), "e2"); err != nil {
		t.Fatalf("expired lease replacement = %v", err)
	}
}

func TestClaimRollsBackWhenRecoveryFails(t *testing.T) {
	t.Parallel()
	persistence := openStore(t)
	o := owner(persistence, "a", func() time.Time { return base })
	boom := errors.New("recovery failed")
	err := ClaimAndRecover(t.Context(), o, "e1", func(*sql.Tx, time.Time) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	held, _ := persistence.Autocommit().HoldsLease(t.Context(), "e1", "a", domain.TimeText(base))
	if held {
		t.Fatal("ownership committed despite the failed recovery")
	}
}

func TestKillReleasesUnstartedReservationsAndStaysTerminal(t *testing.T) {
	t.Parallel()
	persistence := openStore(t)
	epochs := &Epochs{Store: persistence, Now: func() time.Time { return base }}
	if err := Drain(t.Context(), epochs, "e"); err != nil {
		t.Fatal(err)
	}
	if err := AssertAdmission(t.Context(), epochs, "e"); !errors.Is(err, domain.ErrEpochDraining) {
		t.Fatalf("draining admission = %v", err)
	}
	if err := Kill(t.Context(), epochs, "e"); err != nil {
		t.Fatal(err)
	}
	if err := Drain(t.Context(), epochs, "e"); err != nil {
		t.Fatal(err)
	}
	if state, err := State(t.Context(), epochs, "e"); err != nil || state != domain.EpochKilled {
		t.Fatalf("state = %q err=%v", state, err)
	}
	if err := AssertDecision(t.Context(), epochs, "e"); !errors.Is(err, domain.ErrEpochKilled) {
		t.Fatalf("decision = %v", err)
	}
	if err := AssertDecision(t.Context(), epochs, ""); !errors.Is(err, domain.ErrEpochUnbound) {
		t.Fatalf("unbound decision = %v", err)
	}
}

func TestSettlingAnUnknownEpisodeFailsBeforeAnyWrite(t *testing.T) {
	t.Parallel()
	persistence := openStore(t)
	err := persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		return Settle(t.Context(), tx, "missing", 0, "now")
	})
	if err == nil {
		t.Fatal("settlement of a missing reservation accepted")
	}
}

func tenantLimit(t *testing.T, persistence store.Store, tenant string, maxMicro uint64, kill bool) {
	t.Helper()
	err := persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		return SetLimit(t.Context(), tx, domain.TenantScope(tenant), tenant, maxMicro, kill, "2026-01-01T00:00:00Z")
	})
	if err != nil {
		t.Fatal(err)
	}
}

func reserve(persistence store.Store, episode, tenant string, amount uint64) error {
	return persistence.WithTx(context.Background(), func(tx *store.Tx) error {
		return Reserve(context.Background(), tx, episode, tenant, amount, "2026-01-01T00:00:00Z")
	})
}

func TestReservationsRespectCeilingsAndSettleIdempotently(t *testing.T) {
	t.Parallel()
	persistence := openStore(t)
	tenantLimit(t, persistence, "t", 10, false)
	if err := reserve(persistence, "ep1", "t", 6); err != nil {
		t.Fatal(err)
	}
	if err := reserve(persistence, "ep2", "t", 6); !errors.Is(err, domain.ErrCostReservationRejected) {
		t.Fatalf("over-ceiling reservation = %v", err)
	}
	if err := reserve(persistence, "ep3", "t", 0); !errors.Is(err, domain.ErrCostReservationRejected) {
		t.Fatalf("unestimated reservation under an active ceiling = %v", err)
	}
	settle := func(actual uint64) error {
		return persistence.WithTx(t.Context(), func(tx *store.Tx) error {
			return Settle(t.Context(), tx, "ep1", actual, "2026-01-01T00:00:00Z")
		})
	}
	if err := settle(4); err != nil {
		t.Fatal(err)
	}
	if err := settle(4); err != nil {
		t.Fatalf("repeat settlement = %v", err)
	}
	if err := settle(5); err == nil {
		t.Fatal("settlement with a different value accepted")
	}
}

func TestApplyCeilingsKeepsUnsetValuesAndTripsNothing(t *testing.T) {
	t.Parallel()
	persistence := openStore(t)
	tenantLimit(t, persistence, "t", 10, true)
	tenant := uint64(25)
	err := persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		return ApplyCeilings(t.Context(), tx, domain.CostCeilings{Tenant: &tenant}, "t", "2026-01-01T00:00:00Z")
	})
	if err != nil {
		t.Fatal(err)
	}
	maxMicro, kill, err := persistence.Autocommit().ReadLimit(t.Context(), domain.TenantScope("t"))
	if err != nil || maxMicro != 25 || kill {
		t.Fatalf("tenant limit = %d %v %v", maxMicro, kill, err)
	}
}

func TestEpochGatesOnATransaction(t *testing.T) {
	t.Parallel()
	persistence := openStore(t)
	epochs := &Epochs{Store: persistence, Now: func() time.Time { return base }}
	if err := Drain(t.Context(), epochs, "e"); err != nil {
		t.Fatal(err)
	}
	err := persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		if err := AssertOrdinaryTx(t.Context(), epochs, tx, "e"); !errors.Is(err, domain.ErrEpochDraining) {
			t.Errorf("ordinary on draining = %v", err)
		}
		if err := AssertOrdinaryTx(t.Context(), epochs, tx, "other"); err != nil {
			t.Errorf("ordinary on an uncontrolled epoch = %v", err)
		}
		if err := AssertDecisionTx(t.Context(), epochs, tx, "e"); err != nil {
			t.Errorf("decision on draining = %v", err)
		}
		if err := AssertDecisionTx(t.Context(), epochs, tx, ""); !errors.Is(err, domain.ErrEpochUnbound) {
			t.Errorf("unbound decision = %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReleasedLeaseCannotBeAssertedAgain(t *testing.T) {
	t.Parallel()
	persistence := openStore(t)
	o := owner(persistence, "a", func() time.Time { return base })
	if err := Claim(t.Context(), o, "e1"); err != nil {
		t.Fatal(err)
	}
	assertHeld := func() error {
		return persistence.WithTx(t.Context(), func(tx *store.Tx) error { return Assert(t.Context(), o, tx, "e1") })
	}
	if err := assertHeld(); err != nil {
		t.Fatal(err)
	}
	if err := Release(t.Context(), o, "e1"); err != nil {
		t.Fatal(err)
	}
	if err := assertHeld(); !errors.Is(err, domain.ErrRuntimeOwnerBusy) {
		t.Fatalf("released lease assertion = %v", err)
	}
	if err := Release(t.Context(), o, "e2"); !errors.Is(err, domain.ErrRuntimeOwnerBusy) {
		t.Fatalf("releasing another epoch = %v", err)
	}
}

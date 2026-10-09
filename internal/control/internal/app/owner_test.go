package app

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestAnotherInstanceCannotClaimAnUnexpiredLeaseUntilItExpires(t *testing.T) {
	t.Parallel()
	persistence, clock := openStore(t), sources.NewVirtual(base)
	if err := Claim(t.Context(), owner(persistence, "a", clock), "e1"); err != nil {
		t.Fatal(err)
	}
	if err := Claim(t.Context(), owner(persistence, "b", clock), "e2"); !errors.Is(err, domain.ErrRuntimeOwnerBusy) {
		t.Fatalf("second epoch claim = %v, want ErrRuntimeOwnerBusy", err)
	}
	clock.Advance(2 * time.Minute)
	if err := Renew(t.Context(), owner(persistence, "a", clock), "e1"); !errors.Is(err, domain.ErrRuntimeOwnerBusy) {
		t.Fatalf("expired renew = %v, want ErrRuntimeOwnerBusy", err)
	}
	if err := Claim(t.Context(), owner(persistence, "b", clock), "e2"); err != nil {
		t.Fatalf("expired lease replacement = %v", err)
	}
}

func TestClaimRollsBackWhenRecoveryFails(t *testing.T) {
	t.Parallel()
	persistence := openStore(t)
	o := owner(persistence, "a", sources.NewVirtual(base))
	boom := errors.New("recovery failed")
	err := ClaimAndRecover(t.Context(), o, "e1", func(*sql.Tx, time.Time) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	held, err := persistence.Autocommit().HoldsLease(t.Context(), "e1", "a", base)
	if err != nil || held {
		t.Fatalf("ownership committed despite the failed recovery: held=%v err=%v", held, err)
	}
}

func TestReleasedLeaseCannotBeAssertedAgain(t *testing.T) {
	t.Parallel()
	persistence := openStore(t)
	o := owner(persistence, "a", sources.NewVirtual(base))
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

func TestClaimingTheSameEpochAgainRenewsTheLeaseForTheSameInstance(t *testing.T) {
	t.Parallel()
	persistence, clock := openStore(t), sources.NewVirtual(base)
	o := owner(persistence, "a", clock)
	if err := Claim(t.Context(), o, "e1"); err != nil {
		t.Fatal(err)
	}
	clock.Advance(50 * time.Second)
	if err := Claim(t.Context(), o, "e1"); err != nil {
		t.Fatalf("reclaim by the holder: %v", err)
	}
	clock.Advance(50 * time.Second)
	if err := Renew(t.Context(), o, "e1"); err != nil {
		t.Fatalf("the reclaim must have extended the lease past the original end: %v", err)
	}
}

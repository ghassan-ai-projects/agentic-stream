package control_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestOwnerClaimsRenewsAndReleasesTheLeaseBeforeAnotherEpochClaimsIt(t *testing.T) {
	t.Parallel()
	db, clock := openOwnerDB(t), sources.NewVirtual(epoch0)
	owner := ownerOn(db, "instance-1", clock)
	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := owner.Renew(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("renew: %v", err)
	}
	if err := owner.Release(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := fenced(t, db, owner.Assert, "epoch-1"); !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
		t.Fatalf("assert after release = %v, want ErrRuntimeOwnerBusy", err)
	}
	if err := owner.Claim(t.Context(), "epoch-2"); err != nil {
		t.Fatalf("claim after release: %v", err)
	}
}

func TestAnUnexpiredLeaseRefusesAnotherEpochOrInstance(t *testing.T) {
	t.Parallel()
	db, clock := openOwnerDB(t), sources.NewVirtual(epoch0)
	if err := ownerOn(db, "instance-1", clock).Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	second := ownerOn(db, "instance-2", clock)
	if err := second.Claim(t.Context(), "epoch-2"); !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
		t.Fatalf("second epoch claim = %v, want ErrRuntimeOwnerBusy", err)
	}
	if err := second.Claim(t.Context(), "epoch-1"); !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
		t.Fatalf("same epoch from another instance = %v, want ErrRuntimeOwnerBusy", err)
	}
}

func TestALostLeaseFencesTheOldEpochsRenewalsAndWrites(t *testing.T) {
	t.Parallel()
	db, clock := openOwnerDB(t), sources.NewVirtual(epoch0)
	first, second := ownerOn(db, "instance-1", clock), ownerOn(db, "instance-2", clock)
	if err := first.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	clock.Advance(2 * time.Minute)
	if err := second.Claim(t.Context(), "epoch-2"); err != nil {
		t.Fatalf("replacement claim after the lease expired: %v", err)
	}
	if err := first.Renew(t.Context(), "epoch-1"); !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
		t.Fatalf("old epoch renewal = %v, want ErrRuntimeOwnerBusy", err)
	}
	if err := fenced(t, db, first.Assert, "epoch-1"); !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
		t.Fatalf("old epoch write fence = %v, want ErrRuntimeOwnerBusy", err)
	}
	if err := fenced(t, db, second.Assert, "epoch-2"); err != nil {
		t.Fatalf("new epoch write fence = %v", err)
	}
}

func TestAnOwnerThatDoesNotRenewLosesTheLeaseWhenItExpires(t *testing.T) {
	t.Parallel()
	db, clock := openOwnerDB(t), sources.NewVirtual(epoch0)
	owner := ownerOn(db, "instance-1", clock)
	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := fenced(t, db, owner.Assert, "epoch-1"); err != nil {
		t.Fatalf("assert while the lease is live: %v", err)
	}
	clock.Advance(2 * time.Minute)
	if err := fenced(t, db, owner.Assert, "epoch-1"); !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
		t.Fatalf("assert after the lease expired = %v, want ErrRuntimeOwnerBusy", err)
	}
	if err := owner.Renew(t.Context(), "epoch-1"); !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
		t.Fatalf("renewal after the lease expired = %v, want ErrRuntimeOwnerBusy", err)
	}
}

func TestUnsetLeaseIsTheSourcesDefaultLease(t *testing.T) {
	t.Parallel()
	db, clock := openOwnerDB(t), sources.NewVirtual(epoch0)
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-1", Now: clock.Now}
	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	clock.Advance(sources.DefaultLease - time.Nanosecond)
	if err := fenced(t, db, owner.Assert, "epoch-1"); err != nil {
		t.Fatalf("assert one nanosecond before the default lease ends: %v", err)
	}
	clock.Advance(time.Nanosecond)
	if err := fenced(t, db, owner.Assert, "epoch-1"); !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
		t.Fatalf("assert when the default lease ends = %v, want ErrRuntimeOwnerBusy", err)
	}
}

func TestAnUnconfiguredOwnerRefusesToClaimAndFencesNothing(t *testing.T) {
	t.Parallel()
	db := openOwnerDB(t)
	var missing *runtimecontrol.RuntimeOwner
	cases := map[string]struct {
		owner *runtimecontrol.RuntimeOwner
		epoch string
	}{
		"nil owner":   {missing, "epoch-1"},
		"no database": {&runtimecontrol.RuntimeOwner{InstanceID: "instance-1"}, "epoch-1"},
		"no instance": {&runtimecontrol.RuntimeOwner{DB: db}, "epoch-1"},
		"no epoch":    {&runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-1"}, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertRefusal(t, tc.owner.Claim(t.Context(), tc.epoch), "runtime owner is not configured")
			assertRefusal(t, tc.owner.ClaimAndRecover(t.Context(), tc.epoch, nil), "runtime owner is not configured")
		})
	}
	t.Run("renew release and assert need an owner and an epoch", func(t *testing.T) {
		t.Parallel()
		owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-1"}
		for name, err := range map[string]error{
			"renew of a nil owner":    missing.Renew(t.Context(), "epoch-1"),
			"renew without an epoch":  owner.Renew(t.Context(), ""),
			"release of a nil owner":  missing.Release(t.Context(), "epoch-1"),
			"release without epoch":   owner.Release(t.Context(), ""),
			"assert for a nil owner":  fenced(t, db, missing.Assert, "epoch-1"),
			"assert without an epoch": fenced(t, db, owner.Assert, ""),
		} {
			if err == nil || !strings.Contains(err.Error(), "runtime owner is not configured") {
				t.Errorf("%s = %v, want a not-configured refusal", name, err)
			}
		}
	})
}

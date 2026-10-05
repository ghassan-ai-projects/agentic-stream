package authority_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
)

func TestClaimFencesAnotherOwnerUntilTheClaimExpires(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.service.Claim(t.Context(), fanClaim); err != nil {
		t.Fatal(err)
	}
	if err := f.runtime.Release(t.Context(), ownerOne.Epoch); err != nil { // the runtime lease ends; the target claim does not
		t.Fatal(err)
	}
	ownerTwo := authority.Owner{Epoch: "epoch-2", Instance: "instance-2"}
	_, second := f.admit(t, ownerTwo, runtimeLease)
	takeover := authority.TargetClaim{Target: "fan-01", Device: bootB, Owner: ownerTwo}
	if err := second.Claim(t.Context(), takeover); !errors.Is(err, authority.ErrTargetClaimBusy) {
		t.Fatalf("live claim of another owner = %v", err)
	}
	if f.count(t, `SELECT COUNT(*) FROM device_authority_events WHERE event_type = 'claim_rejected'`) != 1 {
		t.Fatal("rejected claim was not audited")
	}
	f.clock.Advance(10 * time.Minute)
	if err := second.Claim(t.Context(), takeover); err != nil {
		t.Fatalf("expired claim was not recoverable: %v", err)
	}
	if err := second.AssertClaim(t.Context(), takeover); err != nil {
		t.Fatalf("taken-over claim did not assert: %v", err)
	}
	if fence := f.count(t, `SELECT claim_fence FROM device_target_claims WHERE target = 'fan-01'`); fence != 2 {
		t.Fatalf("takeover fence = %d, want 2", fence)
	}
}

func TestAssertClaimRequiresTheExactLiveHolder(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.service.Claim(t.Context(), fanClaim); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*authority.TargetClaim){
		func(c *authority.TargetClaim) { c.Device.DeviceID = "other" },
		func(c *authority.TargetClaim) { c.Device.BootID = "other" },
		func(c *authority.TargetClaim) { c.Target = "other" },
	} {
		other := fanClaim
		mutate(&other)
		if err := f.service.AssertClaim(t.Context(), other); !errors.Is(err, authority.ErrTargetClaimNotOwned) {
			t.Fatalf("assert %+v = %v", other, err)
		}
		if err := f.service.ReleaseClaim(t.Context(), other); !errors.Is(err, authority.ErrTargetClaimNotOwned) {
			t.Fatalf("release %+v = %v", other, err)
		}
	}
	if err := f.service.Claim(t.Context(), fanClaim); err != nil { // renewal keeps the claim live
		t.Fatal(err)
	}
	f.clock.Advance(10 * time.Minute)
	if err := f.service.AssertClaim(t.Context(), fanClaim); !errors.Is(err, authority.ErrTargetClaimNotOwned) {
		t.Fatalf("assert at claim expiry = %v", err)
	}
}

func TestReleaseRollsBackWhenItsAuditCannotBeWritten(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.service.Claim(t.Context(), fanClaim); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(t.Context(), `CREATE TRIGGER reject_release_audit
		BEFORE INSERT ON device_authority_events WHEN NEW.event_type = 'claim_released'
		BEGIN SELECT RAISE(ABORT, 'injected audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ReleaseClaim(t.Context(), fanClaim); err == nil || !strings.Contains(err.Error(), "record authority event") {
		t.Fatalf("release audit failure = %v", err)
	}
	if err := f.service.AssertClaim(t.Context(), fanClaim); err != nil {
		t.Fatalf("claim was released without its audit: %v", err)
	}
}

func TestReleaseStaysAvailableAfterAuthorityLoss(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.service.Claim(t.Context(), fanClaim); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(runtimeLease)
	if err := f.service.ReleaseClaim(t.Context(), fanClaim); err != nil {
		t.Fatalf("release after authority loss: %v", err)
	}
	if err := f.service.ReleaseClaim(t.Context(), fanClaim); !errors.Is(err, authority.ErrTargetClaimNotOwned) {
		t.Fatalf("second release = %v", err)
	}
}

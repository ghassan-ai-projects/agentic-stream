package domain

import (
	"errors"
	"testing"
	"time"
)

var (
	testNow   = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	ownerA    = Owner{Epoch: "epoch-a", Instance: "instance-a"}
	ownerB    = Owner{Epoch: "epoch-b", Instance: "instance-b"}
	bootOne   = DeviceBoot{DeviceID: "thermal-01", BootID: "boot-1"}
	claimByA  = TargetClaim{Target: "fan-01", Device: bootOne, Owner: ownerA}
	claimByB  = TargetClaim{Target: "fan-01", Device: bootOne, Owner: ownerB}
	liveLease = testNow.Add(time.Minute)
	pastLease = testNow.Add(-time.Second)
	heldBy    = func(claim TargetClaim, fence int64, lease time.Time, status ClaimStatus) *HeldClaim {
		return &HeldClaim{TargetClaim: claim, Fence: fence, LeaseUntil: lease, Status: status}
	}
)

func TestDecideClaim(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		held  *HeldClaim
		claim TargetClaim
		want  ClaimDecision
	}{
		{"unclaimed target is acquired with the first fence", nil, claimByA, ClaimDecision{EventClaimAcquired, 1}},
		{"live renewal keeps the fence", heldBy(claimByA, 4, liveLease, ClaimActive), claimByA, ClaimDecision{EventClaimRenewed, 4}},
		{"expired renewal advances the fence", heldBy(claimByA, 4, pastLease, ClaimActive), claimByA, ClaimDecision{EventClaimRenewed, 5}},
		{"another owner's live claim rejects", heldBy(claimByB, 4, liveLease, ClaimActive), claimByA, ClaimDecision{Event: EventClaimRejected}},
		{"another owner's expired claim is taken over", heldBy(claimByB, 4, pastLease, ClaimActive), claimByA, ClaimDecision{EventClaimAcquired, 5}},
		{"released claim is reacquired with a new fence", heldBy(claimByA, 4, liveLease, ClaimReleased), claimByA, ClaimDecision{EventClaimAcquired, 5}},
		{"lease ending exactly now is expired", heldBy(claimByB, 4, testNow, ClaimActive), claimByA, ClaimDecision{EventClaimAcquired, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := DecideClaim(tt.held, tt.claim, testNow); got != tt.want {
				t.Fatalf("DecideClaim = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestClaimEventRecordsFenceOrRejection(t *testing.T) {
	t.Parallel()
	held := heldBy(claimByB, 2, liveLease, ClaimActive)
	rejected := ClaimEvent(ClaimDecision{Event: EventClaimRejected}, claimByA, held, testNow)
	if rejected.Type != EventClaimRejected || rejected.Details["current_epoch"] != "epoch-b" || rejected.Details["current_instance"] != "instance-b" {
		t.Fatalf("rejection event = %+v", rejected)
	}
	acquired := ClaimEvent(ClaimDecision{Event: EventClaimAcquired, Fence: 3}, claimByA, nil, testNow)
	if acquired.Details["claim_fence"] != int64(3) || acquired.Subject != claimByA || !acquired.OccurredAt.Equal(testNow) {
		t.Fatalf("acquire event = %+v", acquired)
	}
	if released := ReleaseEvent(claimByA, testNow); released.Type != EventClaimReleased || released.Details == nil {
		t.Fatalf("release event = %+v", released)
	}
}

func TestClaimHolderChecks(t *testing.T) {
	t.Parallel()
	otherBoot := claimByA
	otherBoot.Device.BootID = "boot-2"
	tests := []struct {
		name            string
		held            *HeldClaim
		claim           TargetClaim
		heldErr, relErr error
	}{
		{"exact live holder", heldBy(claimByA, 1, liveLease, ClaimActive), claimByA, nil, nil},
		{"no claim", nil, claimByA, ErrTargetClaimNotOwned, ErrTargetClaimNotOwned},
		{"different owner", heldBy(claimByB, 1, liveLease, ClaimActive), claimByA, ErrTargetClaimNotOwned, ErrTargetClaimNotOwned},
		{"different boot", heldBy(claimByA, 1, liveLease, ClaimActive), otherBoot, ErrTargetClaimNotOwned, ErrTargetClaimNotOwned},
		{"expired but active may still be released", heldBy(claimByA, 1, pastLease, ClaimActive), claimByA, ErrTargetClaimNotOwned, nil},
		{"released", heldBy(claimByA, 1, liveLease, ClaimReleased), claimByA, ErrTargetClaimNotOwned, ErrTargetClaimNotOwned},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := CheckClaimHeld(tt.held, tt.claim, testNow); !errors.Is(err, tt.heldErr) {
				t.Fatalf("CheckClaimHeld = %v, want %v", err, tt.heldErr)
			}
			if err := CheckReleasable(tt.held, tt.claim); !errors.Is(err, tt.relErr) {
				t.Fatalf("CheckReleasable = %v, want %v", err, tt.relErr)
			}
		})
	}
}

func TestIdentityCompleteness(t *testing.T) {
	t.Parallel()
	if !claimByA.Complete() || (TargetClaim{Device: bootOne, Owner: ownerA}).Complete() {
		t.Fatal("target claim completeness")
	}
	if (Owner{Epoch: "e"}).Complete() || (DeviceBoot{DeviceID: "d"}).Complete() {
		t.Fatal("partial identities reported complete")
	}
	if subject := DeviceSubject(bootOne, ownerA); subject.Target != bootOne.DeviceID || subject.Owner != ownerA {
		t.Fatalf("device subject = %+v", subject)
	}
}

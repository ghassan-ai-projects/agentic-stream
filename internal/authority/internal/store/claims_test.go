package store

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
)

func TestAClaimIsWrittenReadBackAndMarkedReleased(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	lease := testNow.Add(time.Minute)
	work(t, s, func(tx *Tx) error {
		return tx.WriteClaim(t.Context(), claim, domain.ClaimDecision{Fence: 3, LeaseUntil: lease}, testNow)
	})
	work(t, s, func(tx *Tx) error {
		held, err := tx.LoadClaim(t.Context(), claim.Target)
		if err != nil || held.TargetClaim != claim || held.Fence != 3 || !held.LeaseUntil.Equal(lease) || held.Status != domain.ClaimActive {
			t.Fatalf("held claim = %+v, %v", held, err)
		}
		return tx.MarkClaimReleased(t.Context(), claim.Target, testNow)
	})
	work(t, s, func(tx *Tx) error {
		if held, err := tx.LoadClaim(t.Context(), claim.Target); err != nil || held.Status != domain.ClaimReleased {
			t.Fatalf("released claim = %+v, %v", held, err)
		}
		return nil
	})
}

func TestAnUnclaimedTargetHasNoClaim(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	work(t, s, func(tx *Tx) error {
		if held, err := tx.LoadClaim(t.Context(), "never-claimed"); err != nil || held != nil {
			t.Fatalf("LoadClaim = %+v, %v; want no claim", held, err)
		}
		return nil
	})
}

func TestTheNextAcceptedDecisionReplacesTheClaimOnATarget(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	takeover := domain.TargetClaim{Target: claim.Target, Device: domain.DeviceBoot{DeviceID: "thermal-02", BootID: "boot-Z"}, Owner: domain.Owner{Epoch: "epoch-2", Instance: "instance-2"}}
	work(t, s, func(tx *Tx) error {
		return tx.WriteClaim(t.Context(), claim, domain.ClaimDecision{Fence: 1, LeaseUntil: testNow.Add(time.Minute)}, testNow)
	})
	later := testNow.Add(time.Hour)
	work(t, s, func(tx *Tx) error {
		return tx.WriteClaim(t.Context(), takeover, domain.ClaimDecision{Fence: 2, LeaseUntil: later.Add(time.Minute)}, later)
	})
	work(t, s, func(tx *Tx) error {
		held, err := tx.LoadClaim(t.Context(), claim.Target)
		if err != nil || held.TargetClaim != takeover || held.Fence != 2 || held.Status != domain.ClaimActive {
			t.Fatalf("claim after takeover = %+v, %v; want the new holder at fence 2", held, err)
		}
		return nil
	})
}

func TestAClaimWithAnUnreadableLeaseIsRefused(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	work(t, s, func(tx *Tx) error {
		return tx.WriteClaim(t.Context(), claim, domain.ClaimDecision{Fence: 1, LeaseUntil: testNow.Add(time.Minute)}, testNow)
	})
	if _, err := db.ExecContext(t.Context(), `UPDATE device_target_claims SET lease_until = 'soon'`); err != nil {
		t.Fatal(err)
	}
	err := s.InTx(t.Context(), func(tx *Tx) error {
		_, err := tx.LoadClaim(t.Context(), claim.Target)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "parse target claim lease") {
		t.Fatalf("LoadClaim = %v, want a refusal naming the lease", err)
	}
}

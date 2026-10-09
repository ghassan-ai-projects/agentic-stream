package app_test

import (
	"errors"
	"testing"
	"time"
)

func deviceEvidence(safeState bool) map[string]any {
	return map[string]any{
		"source": "device.query_state", "evidence_type": "device_state_feedback",
		"device_id": "thermal-01", "boot_id": "boot-A", "state": map[string]any{"safe_state": safeState},
		"state_digest": zeroDigest, "feedback_digest": zeroDigest,
	}
}

func TestDeviceStateDecidesTheOutcomeAfterADispatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		effector *deviceEffector
		want     ledger
	}{
		{"accepted and verified", &deviceEffector{scriptedEffector: *pendingVerification(), finalStatus: "succeeded", evidence: deviceEvidence(false)},
			ledger{Command: "succeeded", Outbox: "delivered", Outcome: "succeeded", Reconciliation: "observed", Verification: "observed", Outcomes: 1}},
		{"unknown outcome resolved by the device state", &deviceEffector{scriptedEffector: *failsWith(unknownOutcome), finalStatus: "failed", evidence: deviceEvidence(true)},
			ledger{Command: "failed", Outbox: "failed", Outcome: "reconciled", Reconciliation: "reconciled", Verification: "refuted", Outcomes: 2}},
		{"state query failure keeps the outcome unknown", &deviceEffector{scriptedEffector: *pendingVerification(), verifyErr: errors.New("state query failed")},
			ledger{Command: "reconciling", Outbox: "failed", Outcome: "unknown", Reconciliation: "required", Verification: "awaiting", Outcomes: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, commandID := openActionFixture(t)
			dispatcher := newDispatcher(t, db, tc.effector)
			if !dispatchOnce(t, dispatcher) {
				t.Fatal("the dispatch found nothing to do")
			}
			if got := readLedger(t, db, commandID); got != tc.want {
				t.Fatalf("ledger = %+v, want %+v", got, tc.want)
			}
			if tc.effector.calls != 1 || tc.effector.verifyCalls != 1 {
				t.Fatalf("dispatch calls = %d, verify calls = %d, want one each", tc.effector.calls, tc.effector.verifyCalls)
			}
		})
	}
}

func TestTheDispatchCallAndTheDeviceQueryBothRunUnderTheDispatchLeaseDeadline(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	effector := &deviceEffector{scriptedEffector: *pendingVerification(), finalStatus: "succeeded", evidence: deviceEvidence(false)}
	lease := time.Minute
	started := time.Now()
	dispatchOnce(t, newDispatcher(t, db, effector, withLease(lease)))
	for name, deadline := range map[string]time.Time{"dispatch call": effector.deadline, "device query": effector.verifyDeadline} {
		if deadline.IsZero() || deadline.After(started.Add(lease)) {
			t.Errorf("%s deadline = %v, want a deadline within %v of the dispatch start", name, deadline, lease)
		}
	}
}

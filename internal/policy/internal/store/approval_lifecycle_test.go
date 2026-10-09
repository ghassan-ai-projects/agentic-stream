package store

import (
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

func approvalStatus(t *testing.T, tx *Tx, approvalID string) string {
	t.Helper()
	approval, err := tx.LoadApproval(t.Context(), approvalID, "tenant")
	if err != nil {
		t.Fatalf("load approval %s: %v", approvalID, err)
	}
	return approval.Status
}

func TestARequestedApprovalIsPendingAndBoundToItsNonceAndExpiry(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		if id, err := tx.PendingApproval(t.Context(), intentID); err != nil || id != "" {
			t.Fatalf("before the request: pending approval %q, %v", id, err)
		}
		requestPendingApproval(t, tx, intentID, "approval-1", fixtureNow.Add(time.Hour), fixtureNow)

		approval, err := tx.LoadApproval(t.Context(), "approval-1", "tenant")
		if err != nil || approval.IntentID != intentID || approval.Status != "pending" || !approval.ExpiresAt.Equal(fixtureNow.Add(time.Hour)) {
			t.Fatalf("approval = %+v, %v", approval, err)
		}
		if id, expiry, err := tx.PendingApprovalExpiry(t.Context(), intentID); err != nil || id != "approval-1" || !expiry.Equal(fixtureNow.Add(time.Hour)) {
			t.Fatalf("pending expiry = %q %v %v", id, expiry, err)
		}
		if expiry, nonce, err := tx.AssertionBinding(t.Context(), "approval-1"); err != nil || nonce != "nonce-approval-1" || !expiry.Equal(fixtureNow.Add(time.Hour)) {
			t.Fatalf("assertion binding = %v %q %v", expiry, nonce, err)
		}
		return nil
	})
}

func TestAnApprovalIsLoadedOnlyForItsOwnTenant(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		requestPendingApproval(t, tx, intentID, "approval-1", fixtureNow.Add(time.Hour), fixtureNow)
		for _, lookup := range []struct{ id, tenant string }{{"approval-1", "other"}, {"absent", "tenant"}} {
			if _, err := tx.LoadApproval(t.Context(), lookup.id, lookup.tenant); !errors.Is(err, domain.ErrApprovalNotFound) {
				t.Errorf("LoadApproval(%q, %q) = %v, want %v", lookup.id, lookup.tenant, err, domain.ErrApprovalNotFound)
			}
		}
		return nil
	})
}

func TestAHumanDecisionLeavesThePendingStateAndKeepsItsOwnRecord(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		requestPendingApproval(t, tx, intentID, "approval-1", fixtureNow.Add(time.Hour), fixtureNow)
		if err := tx.BindAssertion(t.Context(), "approval-1", make([]byte, 32)); err != nil {
			return err
		}
		resolution := domain.ApprovalResolution{ID: "approval-1", Approver: "operator-1", Relay: "relay-1", Reason: "reviewed", Now: fixtureNow}
		if err := tx.ResolveApproval(t.Context(), resolution, "approved", "resolve approval approval-1"); err != nil {
			return err
		}
		if id, err := tx.PendingApproval(t.Context(), intentID); err != nil || id != "" {
			t.Errorf("after the decision: pending approval %q, %v", id, err)
		}
		if id, err := tx.ApprovedApproval(t.Context(), intentID); err != nil || id != "approval-1" {
			t.Errorf("approved approval = %q, %v", id, err)
		}
		return nil
	})
	approver, relay := scalar[string](t, db, "SELECT approver_identity FROM approvals WHERE approval_id = 'approval-1'"), scalar[string](t, db, "SELECT relay_identity FROM approvals WHERE approval_id = 'approval-1'")
	if approver != "operator-1" || relay != "relay-1" {
		t.Fatalf("recorded approver %q via relay %q, want operator-1 via relay-1", approver, relay)
	}
}

func TestExpiryAndWithdrawalAreRecordedAsDistinctOutcomes(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		requestPendingApproval(t, tx, intentID, "expires", fixtureNow, fixtureNow)
		if err := tx.ExpireApproval(t.Context(), "expires", fixtureNow); err != nil {
			return err
		}
		requestPendingApproval(t, tx, intentID, "withdrawn", fixtureNow, fixtureNow)
		if err := tx.WithdrawApproval(t.Context(), "withdrawn", fixtureNow); err != nil {
			return err
		}
		requestPendingApproval(t, tx, intentID, "intent-expires", fixtureNow, fixtureNow)
		if err := tx.ExpireIntentApproval(t.Context(), intentID); err != nil {
			return err
		}
		for id, want := range map[string]string{"expires": "expired", "withdrawn": "denied", "intent-expires": "expired"} {
			if got := approvalStatus(t, tx, id); got != want {
				t.Errorf("approval %s is %q, want %q", id, got, want)
			}
		}
		return nil
	})
	if withdrawn := scalar[bool](t, db, "SELECT withdrawn_at IS NOT NULL FROM approvals WHERE approval_id = 'withdrawn'"); !withdrawn {
		t.Fatal("a withdrawn approval carries no withdrawal time")
	}
}

func TestApprovalLifecycleEventsAreAnnouncedThroughTheNotificationOutbox(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil {
			return err
		}
		snapshot, err := tx.ApprovalSnapshotDigest(t.Context(), row)
		if err != nil {
			return err
		}
		intent, reason := domain.DecodeDocument(row.IntentJSON, contractsv1.SchemaIntent)
		decision, decisionReason := domain.ParseDecision(row)
		if reason != "" || decisionReason != "" {
			t.Fatalf("stored documents: %q %q", reason, decisionReason)
		}
		notice := domain.ApprovalNotice{Row: row, ID: "approval-1", ExpiresAt: fixtureNow.Add(time.Hour), Intent: domain.ProjectIntent(intent), Context: domain.ApprovalContext{Snapshot: snapshot, Decision: decision, Delta: map[string]any{}, Source: tx.NotificationSource(row.TenantID)}}
		request := domain.ApprovalRequest{ID: "approval-1", Nonce: "nonce", Data: domain.BuildApprovalNotification(notice), JSON: []byte("{}")}
		if err := tx.AppendApprovalRequested(t.Context(), row, request, fixtureNow); err != nil {
			t.Errorf("announce request: %v", err)
		}
		event := domain.ApprovalEvent{Intent: row, ID: "approval-1", Status: "approved", Reason: "human", Now: fixtureNow}
		if err := tx.AppendApprovalResolved(t.Context(), event); err != nil {
			t.Errorf("announce resolution: %v", err)
		}
		if err := tx.AppendApprovalWithdrawn(t.Context(), event); err != nil {
			t.Errorf("announce withdrawal: %v", err)
		}
		return nil
	})
	for _, event := range []string{"approval.requested:approval-1", "approval.resolved:approval-1:approved", "approval.withdrawn:approval-1"} {
		if announced := scalar[int](t, db, "SELECT COUNT(*) FROM notifications WHERE event_id = ?", event); announced != 1 {
			t.Errorf("notification %s announced %d times, want once", event, announced)
		}
	}
}

package store

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

func requestPendingApproval(t *testing.T, tx *Tx, intentID, approvalID string, expiresAt, now time.Time) {
	t.Helper()
	request := domain.ApprovalRequest{ID: approvalID, Nonce: "nonce-" + approvalID, JSON: []byte("{}")}
	if err := tx.RequestApproval(t.Context(), domain.ApprovalPublication{IntentID: intentID, Request: request, ExpiresAt: expiresAt, Now: now}); err != nil {
		t.Fatal(err)
	}
}

func TestUnreadableApprovalExpiryRefusesOnlyItsOwnApproval(t *testing.T) {
	t.Parallel()
	now := fixtureNow
	db, intentID := openPolicyFixture(t, "R2", 1, 1, now.Add(time.Hour))
	copyIntent(t, db, intentID, "neighbor-intent", "policy_status = 'pending'")
	if err := db.WithTx(t.Context(), func(original *sql.Tx) error {
		tx := Join(original)
		requestPendingApproval(t, tx, intentID, "corrupt", now.Add(time.Hour), now)
		requestPendingApproval(t, tx, "neighbor-intent", "healthy", now.Add(time.Hour), now)
		if _, err := original.ExecContext(t.Context(), "UPDATE approvals SET expires_at = 'not a time' WHERE approval_id = 'corrupt'"); err != nil {
			return err
		}
		id, expiry, err := tx.PendingApprovalExpiry(t.Context(), intentID)
		if !errors.Is(err, ErrUnreadableApprovalExpiry) || id != "corrupt" || !expiry.IsZero() {
			t.Fatalf("corrupt expiry = %q %v %v", id, expiry, err)
		}
		if expires, nonce, err := tx.AssertionBinding(t.Context(), "corrupt"); err == nil || !expires.IsZero() || nonce != "" {
			t.Fatalf("corrupt binding = %v %q %v, want a refusal", expires, nonce, err)
		}
		id, expiry, err = tx.PendingApprovalExpiry(t.Context(), "neighbor-intent")
		if err != nil || id != "healthy" || !expiry.Equal(now.Add(time.Hour)) {
			t.Fatalf("neighbor expiry = %q %v %v", id, expiry, err)
		}
		if _, nonce, err := tx.AssertionBinding(t.Context(), "healthy"); err != nil || nonce != "nonce-healthy" {
			t.Fatalf("neighbor binding nonce=%q err=%v", nonce, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

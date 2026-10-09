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
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	db, intentID := openPolicyFixture(t, "R2", 1, 1, now.Add(time.Hour))
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(t.Context(), `CREATE TEMP TABLE neighbor AS SELECT * FROM intents WHERE intent_id = ?;
		UPDATE neighbor SET intent_id = 'neighbor-intent'; INSERT INTO intents SELECT * FROM neighbor`, intentID); err != nil {
		t.Fatal(err)
	}
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

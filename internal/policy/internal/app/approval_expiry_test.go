package app_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

func evaluateExisting(t *testing.T, p pendingApproval, now time.Time) (policy.Result, error) {
	t.Helper()
	var result policy.Result
	err := p.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		var err error
		result, err = p.service.EvaluateIntent(t.Context(), tx, policy.EvaluationRequest{IntentID: p.intentID, Now: now})
		return err
	})
	return result, err
}

func TestAnExpiredApprovalIsRecordedExpiredAndRollsBackWithAnyFailedWrite(t *testing.T) {
	t.Parallel()
	failures := map[string]string{
		"approval expiry": "CREATE TRIGGER fail_expiry BEFORE UPDATE ON approvals BEGIN SELECT RAISE(ABORT,'expiry failed'); END",
		"notification":    "CREATE TRIGGER fail_notice BEFORE INSERT ON notifications BEGIN SELECT RAISE(ABORT,'notice failed'); END",
		"intent status":   "CREATE TRIGGER fail_intent BEFORE UPDATE ON intents BEGIN SELECT RAISE(ABORT,'intent failed'); END",
	}
	t.Run("all writes succeed", func(t *testing.T) {
		t.Parallel()
		p := openPendingApproval(t)

		result, err := evaluateExisting(t, p, p.now.Add(2*time.Hour))
		if err != nil || result.Result != "expired" || p.status(t) != "expired" {
			t.Fatalf("result = %+v, %v, status %q; want expired", result, err, p.status(t))
		}
		if commands, outbox := commandAndOutboxCounts(t, p.db); commands != 0 || outbox != 0 {
			t.Fatalf("commands=%d outbox=%d for an expired approval", commands, outbox)
		}
	})
	for failure, trigger := range failures {
		t.Run(failure+" fails", func(t *testing.T) {
			t.Parallel()
			p := openPendingApproval(t)
			exec(t, p.db, trigger)

			if result, err := evaluateExisting(t, p, p.now.Add(2*time.Hour)); err == nil {
				t.Fatalf("result = %+v, want the failed write to fail the evaluation", result)
			}
			if status := p.status(t); status != "pending" {
				t.Fatalf("approval status = %q after a rolled back expiry, want pending", status)
			}
		})
	}
}

func TestAnIntentWithAPendingApprovalDoesNotRequestAnotherOne(t *testing.T) {
	t.Parallel()
	p := openPendingApproval(t)

	result, err := evaluateExisting(t, p, p.now)
	if err != nil || result.ApprovalID != p.approvalID || result.Result != "approval_required" {
		t.Fatalf("result = %+v, %v; want the existing approval %s", result, err, p.approvalID)
	}
	if approvals := scalar[int](t, p.db, "SELECT COUNT(*) FROM approvals"); approvals != 1 {
		t.Fatalf("approvals = %d, want 1", approvals)
	}
}

func TestAnApprovalWithUnreadableExpiryIsExpiredWithAnAuditReason(t *testing.T) {
	t.Parallel()
	p := openPendingApproval(t)
	exec(t, p.db, "UPDATE approvals SET expires_at = 'not a time' WHERE approval_id = ?", p.approvalID)

	result, err := evaluateExisting(t, p, p.now)
	if err != nil || result.Result != "expired" || result.Reason != "approval_expiry_unreadable" || p.status(t) != "expired" {
		t.Fatalf("result = %+v, %v, status %q; want expired/approval_expiry_unreadable", result, err, p.status(t))
	}
}

func TestAnApprovalExpiresExactlyAtItsDeadline(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		offset     time.Duration
		wantResult string
		wantStatus string
	}{
		{"one nanosecond left is live", time.Nanosecond, "approval_required", "pending"},
		{"exactly now is expired", 0, "expired", "expired"},
		{"one nanosecond past is expired", -time.Nanosecond, "expired", "expired"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			p := openPendingApproval(t)
			exec(t, p.db, "UPDATE approvals SET expires_at = ? WHERE approval_id = ?", kernel.FormatTime(p.now.Add(test.offset)), p.approvalID)

			result, err := evaluateExisting(t, p, p.now)
			if err != nil || result.Result != test.wantResult || p.status(t) != test.wantStatus {
				t.Fatalf("result = %+v, %v, status %q; want %s/%s", result, err, p.status(t), test.wantResult, test.wantStatus)
			}
		})
	}
}

func TestResolvingAnApprovalWithUnreadableExpiryIsRefusedAndLeavesItPending(t *testing.T) {
	t.Parallel()
	p := openPendingApproval(t)
	resolution := p.signedResolution(t, true)
	exec(t, p.db, "UPDATE approvals SET expires_at = 'not a time' WHERE approval_id = ?", p.approvalID)

	if result, err := p.resolve(t, resolution); err == nil || errors.Is(err, policy.ErrApprovalUnauthorized) {
		t.Fatalf("resolution = %+v, %v; want a refusal for the unreadable expiry", result, err)
	}
	commands, outbox := commandAndOutboxCounts(t, p.db)
	if status := p.status(t); status != "pending" || commands != 0 || outbox != 0 {
		t.Fatalf("status=%q commands=%d outbox=%d, want pending with nothing dispatched", status, commands, outbox)
	}
}

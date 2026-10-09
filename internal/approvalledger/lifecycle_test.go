package approvalledger_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
)

func TestAnApprovalKeepsItsDecisionAndAssertionBindingThroughLaterTransitions(t *testing.T) {
	t.Parallel()
	db := approvalDB(t)
	digest := make([]byte, 32)
	digest[0] = 7
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		for _, id := range []string{"old", "current", "expired", "denied"} {
			if err := requestApproval(ctx, tx, id, id, "later"); err != nil {
				return err
			}
		}
		if err := approvalledger.BindAssertion(ctx, tx, "old", digest); err != nil {
			return err
		}
		if err := approvalledger.Resolve(ctx, tx, "old", "approved", "human", "relay", "allowed", "decided"); err != nil {
			return err
		}
		if err := approvalledger.Withdraw(ctx, tx, "old", "later"); err != nil {
			return err
		}
		if err := approvalledger.Expire(ctx, tx, "expired", "decided"); err != nil {
			return err
		}
		if err := approvalledger.ExpireIntent(ctx, tx, "current"); err != nil {
			return err
		}
		return approvalledger.Withdraw(ctx, tx, "denied", "decided")
	})
	for id, want := range map[string]string{"old": "approved", "current": "expired", "expired": "expired", "denied": "denied"} {
		if got := statusOf(t, db, id); got != want {
			t.Errorf("%s status=%s want=%s", id, got, want)
		}
	}
	var nonce, approver, relay string
	var bound []byte
	if err := db.QueryRowContext(t.Context(), "SELECT nonce,assertion_sha256,approver_identity,relay_identity FROM approvals WHERE approval_id='old'").Scan(&nonce, &bound, &approver, &relay); err != nil {
		t.Fatal(err)
	}
	if nonce != "nonce-old" || len(bound) != 32 || bound[0] != 7 || approver != "human" || relay != "relay" {
		t.Fatalf("assertion/principal binding changed: nonce=%s digest=%x human=%s relay=%s", nonce, bound, approver, relay)
	}
}

func TestEveryApprovalOfAnIntentIsExplainableThroughTheFacade(t *testing.T) {
	t.Parallel()
	db := approvalDB(t)
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		if err := approvalledger.Request(ctx, tx, "a-expired", "old", "t0", "t1", []byte(`{}`), "n1"); err != nil {
			return err
		}
		if err := approvalledger.Expire(ctx, tx, "a-expired", "t2"); err != nil {
			return err
		}
		if err := approvalledger.Request(ctx, tx, "a-withdrawn", "old", "t3", "t9", []byte(`{}`), "n2"); err != nil {
			return err
		}
		return approvalledger.Withdraw(ctx, tx, "a-withdrawn", "t4")
	})
	views, err := approvalledger.Approvals(t.Context(), db.DB, "old")
	if err != nil || len(views) != 2 {
		t.Fatalf("approvals = %+v err=%v, want both requests", views, err)
	}
	if v := views[0]; v.ApprovalID != "a-expired" || v.Status != "expired" || v.Reason != "approval_expired" || v.DecidedAt != "t2" {
		t.Errorf("expired approval = %+v", v)
	}
	if v := views[1]; v.ApprovalID != "a-withdrawn" || v.Status != "denied" || v.Reason != "approval_withdrawn" || v.WithdrawnAt != "t4" || v.WithdrawalReason != "situation_version_conflict" {
		t.Errorf("withdrawn approval = %+v", v)
	}
}

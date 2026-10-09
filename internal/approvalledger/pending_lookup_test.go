package approvalledger_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
)

func approve(ctx context.Context, tx *sql.Tx, id, decidedAt string) error {
	return approvalledger.Resolve(ctx, tx, id, "approved", "operator", "relay", "ok", decidedAt)
}

func TestAPendingApprovalIsLookedUpWithItsBindingUntilItIsResolved(t *testing.T) {
	t.Parallel()
	db := approvalDB(t)
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		if _, found, err := approvalledger.PendingOfIntent(ctx, tx, "old"); err != nil || found {
			t.Fatalf("pending before request found=%t err=%v", found, err)
		}
		if err := requestApproval(ctx, tx, "first", "old", "t5"); err != nil {
			return err
		}
		pending, found, err := approvalledger.PendingOfIntent(ctx, tx, "old")
		if err != nil || !found || pending.ID != "first" || pending.ExpiresAt != "t5" {
			t.Fatalf("pending = %+v found=%t err=%v", pending, found, err)
		}
		binding, err := approvalledger.PendingBinding(ctx, tx, "first")
		if err != nil || binding.ExpiresAt != "t5" || binding.Nonce != "nonce-first" {
			t.Fatalf("binding = %+v err=%v", binding, err)
		}
		if err := approve(ctx, tx, "first", "t1"); err != nil {
			return err
		}
		if _, found, err := approvalledger.PendingOfIntent(ctx, tx, "old"); err != nil || found {
			t.Fatalf("pending after approval found=%t err=%v", found, err)
		}
		if _, err := approvalledger.PendingBinding(ctx, tx, "first"); err == nil {
			t.Fatal("binding of a resolved approval must fail")
		}
		approved, found, err := approvalledger.LatestApprovedOfIntent(ctx, tx, "old")
		if err != nil || !found || approved.ID != "first" || approved.ExpiresAt != "t5" {
			t.Fatalf("latest approved = %+v found=%t err=%v", approved, found, err)
		}
		return nil
	})
}

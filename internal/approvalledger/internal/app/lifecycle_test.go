package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
)

func TestLifecycleTransitionsUseTheStableReasons(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		seedSituation(t, ctx, raw)
		must(t, Expire(ctx, tx, "current", "t"))
		must(t, Withdraw(ctx, tx, "old", "t"))
		for id, want := range map[string]string{
			"old":     "denied|approval_withdrawn|situation_version_conflict",
			"current": "expired|approval_expired|-",
		} {
			got := queryText(t, ctx, raw, "SELECT status || '|' || reason || '|' || COALESCE(withdrawal_reason, '-') FROM approvals WHERE approval_id = ?", id)
			if got != want {
				t.Errorf("%s = %s, want %s", id, got, want)
			}
		}
	})
}

func TestAWithdrawnApprovalCannotBeApprovedAfterwards(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		seedSituation(t, ctx, raw)
		must(t, Withdraw(ctx, tx, "old", "t1"))
		must(t, Resolve(ctx, tx, "old", domain.StatusApproved, "human", "relay", "late approval", "t2"))
		got := queryText(t, ctx, raw, "SELECT status || '|' || reason || '|' || decided_at || '|' || COALESCE(approver_identity, '-') FROM approvals WHERE approval_id = 'old'")
		if want := "denied|approval_withdrawn|t1|-"; got != want {
			t.Fatalf("approval = %s, want %s: the late approval must change nothing", got, want)
		}
	})
}

func TestAnExpiredApprovalCannotBeWithdrawnOrResolved(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		seedSituation(t, ctx, raw)
		must(t, Expire(ctx, tx, "old", "t1"))
		must(t, Withdraw(ctx, tx, "old", "t2"))
		must(t, Resolve(ctx, tx, "old", domain.StatusApproved, "human", "relay", "late", "t2"))
		must(t, ExpireIntent(ctx, tx, "old"))
		if got := queryText(t, ctx, raw, "SELECT status || '|' || reason || '|' || decided_at FROM approvals WHERE approval_id = 'old'"); got != "expired|approval_expired|t1" {
			t.Fatalf("approval = %s, want it to stay as expired at t1", got)
		}
	})
}

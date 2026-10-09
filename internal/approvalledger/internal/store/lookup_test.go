package store_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
)

func TestAnIntentsPendingApprovalIsFoundOnlyWhileUnresolved(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		if _, found, err := tx.PendingOfIntent(ctx, "i-1"); err != nil || found {
			t.Fatalf("before any request: found=%v err=%v", found, err)
		}
		request(t, ctx, tx, "a-1", "i-1")
		pending, found, err := tx.PendingOfIntent(ctx, "i-1")
		if want := (domain.Approval{ID: "a-1", ExpiresAt: "t9"}); err != nil || !found || pending != want {
			t.Fatalf("PendingOfIntent = %+v found=%v err=%v, want %+v", pending, found, err, want)
		}
		if _, found, err := tx.PendingOfIntent(ctx, "i-other"); err != nil || found {
			t.Fatalf("another intent's pending approval: found=%v err=%v", found, err)
		}
		must(t, tx.DecidePending(ctx, "a-1", "approved", "human", "relay", "ok", "t1"))
		if _, found, err := tx.PendingOfIntent(ctx, "i-1"); err != nil || found {
			t.Fatalf("after the decision: found=%v err=%v", found, err)
		}
	})
}

func TestAPendingApprovalBindsItsExpiryAndNonceUntilItIsDecided(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		if _, err := tx.PendingBinding(ctx, "missing"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("binding of an unknown approval = %v, want ErrNoRows", err)
		}
		request(t, ctx, tx, "a-1", "i-1")
		binding, err := tx.PendingBinding(ctx, "a-1")
		if want := (domain.AssertionBinding{ExpiresAt: "t9", Nonce: "nonce-a-1"}); err != nil || binding != want {
			t.Fatalf("PendingBinding = %+v err=%v, want %+v", binding, err, want)
		}
		must(t, tx.DecidePending(ctx, "a-1", "denied", "human", "relay", "no", "t1"))
		if _, err := tx.PendingBinding(ctx, "a-1"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("binding of a decided approval = %v, want ErrNoRows: the nonce is single-use", err)
		}
	})
}

func TestTheLatestApprovedApprovalIsTheLastDecidedAndTiesTakeTheLargerId(t *testing.T) {
	t.Parallel()
	type approval struct{ id, decidedAt string }
	for name, tc := range map[string]struct {
		approvals []approval
		pending   []string
		want      string
	}{
		"no approval":                               {want: ""},
		"a pending approval is not approved":        {pending: []string{"a"}, want: ""},
		"the later decision wins":                   {approvals: []approval{{"a", "t2"}, {"b", "t1"}}, want: "a"},
		"a tie takes the larger id, inserted first": {approvals: []approval{{"z", "t1"}, {"a", "t1"}}, want: "z"},
		"a tie takes the larger id, inserted last":  {approvals: []approval{{"a", "t1"}, {"z", "t1"}}, want: "z"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
				for i, a := range tc.approvals {
					intent := "intent"
					if i > 0 {
						must(t, tx.ExpirePendingOfIntent(ctx, intent))
					}
					request(t, ctx, tx, a.id, intent)
					must(t, tx.DecidePending(ctx, a.id, "approved", "human", "relay", "ok", a.decidedAt))
				}
				for _, id := range tc.pending {
					request(t, ctx, tx, id, "intent")
				}
				got, found, err := tx.LatestApprovedOfIntent(ctx, "intent")
				if err != nil || found != (tc.want != "") || got.ID != tc.want || (found && got.ExpiresAt != "t9") {
					t.Fatalf("latest approved = %+v found=%v err=%v, want %q", got, found, err, tc.want)
				}
			})
		})
	}
}

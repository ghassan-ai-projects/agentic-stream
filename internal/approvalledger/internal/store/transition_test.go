package store_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
)

func TestARequestedApprovalIsPendingWithItsDocumentAndSingleUseNonce(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		must(t, tx.InsertPending(ctx, "a1", "i1", "t0", "t9", []byte(`{"digest":"d"}`), "nonce-1"))
		if got, want := approvalRow(t, ctx, raw, "a1"), "pending|-|-|-|-|-|-|-|nonce-1"; got != want {
			t.Fatalf("approval row = %s, want %s", got, want)
		}
		row := queryText(t, ctx, raw, "SELECT intent_id || '|' || requested_at || '|' || expires_at || '|' || CAST(approval_json AS TEXT) FROM approvals WHERE approval_id = 'a1'")
		if want := `i1|t0|t9|{"digest":"d"}`; row != want {
			t.Fatalf("request row = %s, want %s", row, want)
		}
	})
}

func TestAnIntentHasOnePendingApprovalAtATimeAndApprovalIdsAreUnique(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		request(t, ctx, tx, "a1", "i1")
		if err := tx.InsertPending(ctx, "a2", "i1", "t0", "t9", []byte("{}"), "n2"); err == nil || !strings.Contains(err.Error(), "write approval") {
			t.Errorf("a second pending approval for an intent = %v, want it refused", err)
		}
		if err := tx.InsertPending(ctx, "a1", "i2", "t0", "t9", []byte("{}"), "n3"); err == nil {
			t.Error("a repeated approval id was accepted")
		}
		must(t, tx.ExpirePendingOfIntent(ctx, "i1"))
		must(t, tx.InsertPending(ctx, "a3", "i1", "t0", "t9", []byte("{}"), "n4"))
	})
}

func TestEachTransitionRecordsItsOwnColumnsAndStableReason(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		apply func(context.Context, *store.Tx) error
		want  string
	}{
		"approve": {func(ctx context.Context, tx *store.Tx) error {
			return tx.DecidePending(ctx, "a1", "approved", "human", "relay", "looks right", "t1")
		}, "approved|t1|human|relay|looks right|-|-|-|nonce-a1"},
		"deny": {func(ctx context.Context, tx *store.Tx) error {
			return tx.DecidePending(ctx, "a1", "denied", "human", "relay", "no", "t1")
		}, "denied|t1|human|relay|no|-|-|-|nonce-a1"},
		"expire": {func(ctx context.Context, tx *store.Tx) error {
			return tx.ExpirePending(ctx, "a1", "t1", domain.ReasonExpired)
		}, "expired|t1|-|-|approval_expired|-|-|-|nonce-a1"},
		"withdraw": {func(ctx context.Context, tx *store.Tx) error {
			return tx.WithdrawPending(ctx, "a1", "t1", domain.WithdrawalConflict, domain.ReasonWithdrawn)
		}, "denied|t1|-|-|approval_withdrawn|t1|situation_version_conflict|-|nonce-a1"},
		"expire with the intent": {func(ctx context.Context, tx *store.Tx) error {
			return tx.ExpirePendingOfIntent(ctx, "i1")
		}, "expired|-|-|-|-|-|-|-|nonce-a1"},
		"bind assertion": {func(ctx context.Context, tx *store.Tx) error {
			return tx.BindAssertionDigest(ctx, "a1", make([]byte, 32))
		}, "pending|-|-|-|-|-|-|0000000000000000000000000000000000000000000000000000000000000000|nonce-a1"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
				request(t, ctx, tx, "a1", "i1")
				must(t, tc.apply(ctx, tx))
				if got := approvalRow(t, ctx, raw, "a1"); got != tc.want {
					t.Fatalf("approval row = %s, want %s", got, tc.want)
				}
			})
		})
	}
}

func TestOnlyAPendingApprovalCanTransition(t *testing.T) {
	t.Parallel()
	startStates := map[string]func(context.Context, *store.Tx) error{
		"approved": func(ctx context.Context, tx *store.Tx) error {
			return tx.DecidePending(ctx, "a1", "approved", "h", "r", "ok", "t1")
		},
		"denied": func(ctx context.Context, tx *store.Tx) error {
			return tx.DecidePending(ctx, "a1", "denied", "h", "r", "no", "t1")
		},
		"expired": func(ctx context.Context, tx *store.Tx) error {
			return tx.ExpirePending(ctx, "a1", "t1", domain.ReasonExpired)
		},
		"withdrawn": func(ctx context.Context, tx *store.Tx) error {
			return tx.WithdrawPending(ctx, "a1", "t1", domain.WithdrawalConflict, domain.ReasonWithdrawn)
		},
	}
	for start, decide := range startStates {
		t.Run(start, func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
				request(t, ctx, tx, "a1", "i1")
				must(t, decide(ctx, tx))
				before := approvalRow(t, ctx, raw, "a1")
				later := map[string]error{
					"approve":  tx.DecidePending(ctx, "a1", "approved", "other", "other", "late", "t2"),
					"deny":     tx.DecidePending(ctx, "a1", "denied", "other", "other", "late", "t2"),
					"expire":   tx.ExpirePending(ctx, "a1", "t2", domain.ReasonExpired),
					"withdraw": tx.WithdrawPending(ctx, "a1", "t2", "x", "y"),
					"intent":   tx.ExpirePendingOfIntent(ctx, "i1"),
					"bind":     tx.BindAssertionDigest(ctx, "a1", make([]byte, 32)),
				}
				for name, err := range later {
					if err != nil {
						t.Errorf("%s on a %s approval: %v", name, start, err)
					}
				}
				if after := approvalRow(t, ctx, raw, "a1"); after != before {
					t.Fatalf("a transition changed a %s approval:\nbefore %s\nafter  %s", start, before, after)
				}
			})
		})
	}
}

func TestATransitionOfAnUnknownApprovalChangesNothing(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		request(t, ctx, tx, "a1", "i1")
		before := approvalRow(t, ctx, raw, "a1")
		must(t, tx.DecidePending(ctx, "missing", "approved", "h", "r", "ok", "t1"))
		must(t, tx.ExpirePending(ctx, "missing", "t1", domain.ReasonExpired))
		must(t, tx.WithdrawPending(ctx, "missing", "t1", "x", "y"))
		must(t, tx.ExpirePendingOfIntent(ctx, "missing"))
		if after := approvalRow(t, ctx, raw, "a1"); after != before {
			t.Fatalf("a transition of another approval changed a1: %s", after)
		}
	})
}

func TestAnApprovalStatusOutsideTheLifecycleIsRefusedByTheSchema(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		request(t, ctx, tx, "a1", "i1")
		err := tx.DecidePending(ctx, "a1", "maybe", "h", "r", "ok", "t1")
		if err == nil || !strings.Contains(err.Error(), "write approval") || errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("DecidePending(maybe) = %v, want the status CHECK to refuse it", err)
		}
		if got := approvalRow(t, ctx, raw, "a1"); !strings.HasPrefix(got, "pending|") {
			t.Fatalf("a refused decision changed the approval: %s", got)
		}
	})
}

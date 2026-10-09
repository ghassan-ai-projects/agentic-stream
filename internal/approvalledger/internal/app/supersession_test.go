package app

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
)

func TestSupersededApprovalsAreWithdrawnThenPublishedInOrder(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		seedSituation(t, ctx, raw)
		var published []domain.Withdrawal
		publish := func(_ context.Context, publishTx *sql.Tx, w domain.Withdrawal) error {
			state := queryText(t, ctx, publishTx, "SELECT status FROM approvals WHERE approval_id = ?", w.ApprovalID)
			if state != domain.StatusDenied {
				t.Errorf("published before withdrawing %s: %s", w.ApprovalID, state)
			}
			if publishTx != raw {
				t.Error("the publisher ran on another transaction than the withdrawal")
			}
			published = append(published, w)
			return nil
		}
		must(t, WithdrawSuperseded(ctx, tx, "situation", 2, "now", publish))
		want := []domain.Withdrawal{{ApprovalID: "old", IntentID: "old", SituationID: "situation", SituationVersion: 1, Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"}}
		if !slices.Equal(published, want) {
			t.Fatalf("published = %+v, want %+v", published, want)
		}
		if state := queryText(t, ctx, raw, "SELECT status FROM approvals WHERE approval_id = 'current'"); state != domain.StatusPending {
			t.Fatalf("the approval of the replacement version is %s, want pending", state)
		}
	})
}

func TestOnlyApprovalsBoundToAStrictlyOlderVersionAreSuperseded(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		replacement int
		want        []string
	}{
		"the same version supersedes nothing":   {1, nil},
		"the next version supersedes version 1": {2, []string{"old"}},
		"a later version supersedes both":       {3, []string{"current", "old"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
				seedSituation(t, ctx, raw)
				var withdrawn []string
				must(t, WithdrawSuperseded(ctx, tx, "situation", tc.replacement, "now", func(_ context.Context, _ *sql.Tx, w domain.Withdrawal) error {
					withdrawn = append(withdrawn, w.ApprovalID)
					return nil
				}))
				if !slices.Equal(withdrawn, tc.want) {
					t.Errorf("replacement version %d withdrew %v, want %v", tc.replacement, withdrawn, tc.want)
				}
			})
		})
	}
}

func TestAFailedPublicationFailsTheWithdrawalAndNamesTheApproval(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		seedSituation(t, ctx, raw)
		boom := errors.New("publication failed")
		calls := 0
		err := WithdrawSuperseded(ctx, tx, "situation", 3, "now", func(context.Context, *sql.Tx, domain.Withdrawal) error {
			calls++
			return boom
		})
		if !errors.Is(err, boom) || calls != 1 {
			t.Fatalf("err = %v after %d publications, want the publication failure and a stop at the first", err, calls)
		}
	})
}

func TestAWithdrawalWithoutAPublisherIsRefusedBeforeAnythingIsWithdrawn(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		seedSituation(t, ctx, raw)
		if err := WithdrawSuperseded(ctx, tx, "situation", 2, "now", nil); !errors.Is(err, ErrPublisherRequired) {
			t.Fatalf("nil publisher = %v, want ErrPublisherRequired", err)
		}
		if state := queryText(t, ctx, raw, "SELECT status FROM approvals WHERE approval_id = 'old'"); state != domain.StatusPending {
			t.Fatalf("a refused withdrawal left the approval %s", state)
		}
	})
}

func TestWithdrawingNothingNeverCallsThePublisher(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		must(t, WithdrawSuperseded(ctx, tx, "situation", 9, "now", func(context.Context, *sql.Tx, domain.Withdrawal) error {
			t.Error("the publisher ran with nothing to withdraw")
			return nil
		}))
	})
}

package store_test

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
)

func seedIntent(t *testing.T, ctx context.Context, raw *sql.Tx, intent, situation string, version int, decision string) {
	t.Helper()
	execSQL(t, ctx, raw, `INSERT INTO intents (intent_id, decision_id, tenant_id, situation_id, situation_version, intent_type, risk_class, intent_json, intent_sha256, expires_at, policy_status, created_at, updated_at)
		VALUES (?, ?, 'tenant', ?, ?, 'review', 'R2', X'7B7D', ?, 'later', 'approval_required', 'now', 'now')`, intent, decision, situation, version, make([]byte, 32))
}

func seedDecision(t *testing.T, ctx context.Context, raw *sql.Tx, decision string, traceparent any) {
	t.Helper()
	execSQL(t, ctx, raw, `INSERT INTO decisions (decision_id, episode_id, attempt_id, fence, ordinal, situation_id, situation_version, raw_json, decision_sha256, validation_status, validation_json, created_at, traceparent, tracestate)
		VALUES (?, ?, ?, 1, 1, 'situation', 1, X'7B7D', ?, 'accepted', X'7B7D', 'now', ?, ?)`, decision, "episode-"+decision, "attempt-"+decision, []byte(decision + "-padding-padding-padding-padding")[:32], traceparent, "vendor=state")
}

func TestSupersededApprovalsAreThePendingOnesBoundToAnOlderVersionOfTheSituation(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		seedDecision(t, ctx, raw, "d-traced", "00-trace-01")
		seedDecision(t, ctx, raw, "d-untraced", nil)
		for _, in := range []struct {
			intent, situation string
			version           int
			decision          string
		}{
			{"i-v1-b", "situation", 1, "d-untraced"}, {"i-v1-a", "situation", 1, "d-traced"}, {"i-v2", "situation", 2, "d-traced"},
			{"i-decided", "situation", 1, "d-traced"}, {"i-elsewhere", "other", 1, "d-traced"},
		} {
			seedIntent(t, ctx, raw, in.intent, in.situation, in.version, in.decision)
			request(t, ctx, tx, "apr-"+in.intent, in.intent)
		}
		must(t, tx.DecidePending(ctx, "apr-i-decided", "approved", "human", "relay", "ok", "t1"))

		got, err := tx.SupersededApprovals(ctx, "situation", 2)
		want := []domain.Withdrawal{
			{ApprovalID: "apr-i-v1-a", IntentID: "i-v1-a", SituationID: "situation", SituationVersion: 1, Traceparent: "00-trace-01", Tracestate: "vendor=state"},
			{ApprovalID: "apr-i-v1-b", IntentID: "i-v1-b", SituationID: "situation", SituationVersion: 1, Tracestate: "vendor=state"},
		}
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("superseded approvals = %+v err=%v, want %+v", got, err, want)
		}
		for _, replacement := range []int{1, 3} {
			got, err := tx.SupersededApprovals(ctx, "situation", replacement)
			if err != nil || (replacement == 1) != (len(got) == 0) || (replacement == 3) != (len(got) == 3) {
				t.Errorf("replacement version %d: %+v err=%v, want strictly older versions only", replacement, got, err)
			}
		}
	})
}

func TestSupersededApprovalsAreEmptyWithoutIntents(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		withdrawals, err := tx.SupersededApprovals(ctx, "situation", 2)
		if err != nil || len(withdrawals) != 0 {
			t.Fatalf("withdrawals=%v err=%v", withdrawals, err)
		}
	})
}

package policy

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
)

// P4: the catalog's per-intent hourly rate limit is enforced before dispatch.
// Dispatches up to the limit pass; one more is denied with rate_limited —
// never clamped or delayed.
func TestEvaluateIntentEnforcesTheCatalogRateLimit(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		limit      int
		dispatches int
		wantReason string
		wantDenied bool
	}{
		{name: "under the limit", limit: 2, dispatches: 0, wantReason: "automatic_r0_r1"},
		{name: "the current dispatch fills the limit", limit: 2, dispatches: 1, wantReason: "automatic_r0_r1"},
		{name: "over the limit", limit: 2, dispatches: 2, wantReason: "rate_limited", wantDenied: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, intentID := openPolicyFixture(t, "R1", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
			defer func() { _ = db.Close() }()
			if _, err := db.ExecContext(ctx,
				"UPDATE intents SET rate_limit_per_hour = ? WHERE intent_id = ?",
				test.limit, intentID); err != nil {
				t.Fatalf("set rate limit: %v", err)
			}
			if test.dispatches > 0 {
				bucket := now.UTC().Format("2006-01-02T15:00")
				if _, err := db.ExecContext(ctx, `
					INSERT INTO intent_dispatch_counts (tenant_id, intent_type, bucket, count)
					VALUES ('tenant', 'create_ticket', ?, ?)`,
					bucket, test.dispatches); err != nil {
					t.Fatalf("insert dispatch: %v", err)
				}
			}

			gateway := NewGateway("policy-v1", ids.Deterministic())
			var result Result
			if err := db.WithTx(ctx, func(tx *sql.Tx) error {
				var err error
				result, err = gateway.EvaluateIntent(ctx, tx, intentID, now)
				return err
			}); err != nil {
				t.Fatalf("evaluate intent: %v", err)
			}
			if test.wantDenied {
				if result.Result != "denied" || result.Reason != "rate_limited" {
					t.Fatalf("result = %s/%s, want denied/rate_limited", result.Result, result.Reason)
				}
				return
			}
			if result.Reason != test.wantReason {
				t.Fatalf("reason = %s, want %s", result.Reason, test.wantReason)
			}
		})
	}
}

// P4: the catalog's declared policy is enforced — an intent the catalog marks
// requires_approval goes through the approval pipeline regardless of risk.
func TestEvaluateIntentHonorsTheCatalogApprovalPolicy(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	db, intentID := openPolicyFixture(t, "R1", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx,
		"UPDATE intents SET requires_approval = 1 WHERE intent_id = ?", intentID); err != nil {
		t.Fatalf("set requires_approval: %v", err)
	}

	gateway := NewGateway("policy-v1", ids.Deterministic())
	var result Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = gateway.EvaluateIntent(ctx, tx, intentID, now)
		return err
	}); err != nil {
		t.Fatalf("evaluate intent: %v", err)
	}
	if result.Result != "approval_required" || result.ApprovalID == "" {
		t.Fatalf("result = %+v, want an approval request", result)
	}
}

var _ = sql.ErrNoRows

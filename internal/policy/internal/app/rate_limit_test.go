package app_test

import (
	"testing"
)

// The catalog's per-intent hourly rate limit is enforced before dispatch:
// dispatches up to the limit pass, one more is denied with rate_limited, never
// clamped or delayed, and a denial does not consume budget.
func TestEvaluationEnforcesTheCatalogRateLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		dispatched    int
		wantResult    string
		wantReason    string
		wantDispatchs int
		wantCommands  int
	}{
		{name: "under the limit", dispatched: 0, wantResult: "approved", wantReason: "automatic_r0_r1", wantDispatchs: 1, wantCommands: 1},
		{name: "the current dispatch fills the limit", dispatched: 1, wantResult: "approved", wantReason: "automatic_r0_r1", wantDispatchs: 2, wantCommands: 1},
		{name: "over the limit", dispatched: 2, wantResult: "denied", wantReason: "rate_limited", wantDispatchs: 2, wantCommands: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			db, intentID := openPolicyFixture(t, "R1", 1, 1, farFuture)
			exec(t, db, "UPDATE intents SET rate_limit_per_hour = 2 WHERE intent_id = ?", intentID)
			bucket := fixtureNow.Format("2006-01-02T15:00")
			if test.dispatched > 0 {
				exec(t, db, "INSERT INTO intent_dispatch_counts (tenant_id, intent_type, bucket, count) VALUES ('tenant', 'create_ticket', ?, ?)", bucket, test.dispatched)
			}

			result := evaluateIntent(t, db, newTestService(t), intentID, fixtureNow)
			if result.Result != test.wantResult || result.Reason != test.wantReason {
				t.Fatalf("result = %s/%s, want %s/%s", result.Result, result.Reason, test.wantResult, test.wantReason)
			}
			dispatched := scalar[int](t, db, "SELECT COALESCE(SUM(count), 0) FROM intent_dispatch_counts WHERE tenant_id = 'tenant' AND intent_type = 'create_ticket' AND bucket = ?", bucket)
			commands, outbox := commandAndOutboxCounts(t, db)
			if dispatched != test.wantDispatchs || commands != test.wantCommands || outbox != test.wantCommands {
				t.Fatalf("dispatch count=%d commands=%d outbox=%d, want %d/%d/%d", dispatched, commands, outbox, test.wantDispatchs, test.wantCommands, test.wantCommands)
			}
		})
	}
}

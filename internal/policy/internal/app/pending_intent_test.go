package app_test

import (
	"fmt"
	"testing"
)

// Raw events and stored documents are evidence, never instructions: policy
// refuses to act on an intent whose stored records do not verify.
func TestAnIntentWhoseStoredRecordsDoNotVerifyIsDeniedBeforeAnyCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		change     string
		wantReason string
	}{
		{"the decision was not accepted", "UPDATE decisions SET validation_status = 'rejected'", "decision_not_accepted"},
		{"the decision digest does not match its bytes", "UPDATE decisions SET decision_sha256 = zeroblob(32)", "decision_digest_mismatch"},
		{"the intent digest does not match its bytes", "UPDATE intents SET intent_sha256 = zeroblob(32)", "intent_digest_mismatch"},
		{"the intent bytes are not an intent", "UPDATE intents SET intent_json = x'7b7d'", "schema_invalid"},
		{"the intent row names another risk class than the signed document", "UPDATE intents SET risk_class = 'R0'", "identity_mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			db, intentID := openPolicyFixture(t, "R1", 1, 1, farFuture)
			exec(t, db, test.change)

			result := evaluateIntent(t, db, newTestService(t), intentID, fixtureNow)
			if result.Result != "denied" || result.Reason != test.wantReason {
				t.Fatalf("result = %s/%s, want denied/%s", result.Result, result.Reason, test.wantReason)
			}
			if commands, outbox := commandAndOutboxCounts(t, db); commands != 0 || outbox != 0 {
				t.Fatalf("commands=%d outbox=%d for a denied intent", commands, outbox)
			}
		})
	}
}

func TestACompensatingIntentMustNameACommandOfItsOwnTenant(t *testing.T) {
	t.Parallel()
	const insertTarget = `
		PRAGMA foreign_keys = OFF;
		INSERT INTO commands (command_id, intent_id, tenant_id, effector_route, normalized_target, idempotency_key, command_json, command_sha256, status, created_at, updated_at)
		VALUES ('cmd-target', 'int-earlier', '%s', 'create_ticket', 'motor/1', zeroblob(32), x'7b7d', zeroblob(32), 'succeeded', 'now', 'now')`
	tests := []struct {
		name       string
		target     string
		wantResult string
		wantReason string
	}{
		{name: "the command does not exist", wantResult: "denied", wantReason: "compensation_target_missing"},
		{name: "the command belongs to another tenant", target: "other", wantResult: "denied", wantReason: "compensation_tenant_mismatch"},
		{name: "the command belongs to the tenant", target: "tenant", wantResult: "approved", wantReason: "automatic_r0_r1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			db, intentID := openPolicyFixture(t, "R1", 1, 1, farFuture)
			resealIntent(t, db, intentID, func(intent map[string]any) { intent["compensates"] = "cmd-target" })
			if test.target != "" {
				exec(t, db, fmt.Sprintf(insertTarget, test.target))
			}

			result := evaluateIntent(t, db, newTestService(t), intentID, fixtureNow)
			if result.Result != test.wantResult || result.Reason != test.wantReason {
				t.Fatalf("result = %s/%s, want %s/%s", result.Result, result.Reason, test.wantResult, test.wantReason)
			}
		})
	}
}

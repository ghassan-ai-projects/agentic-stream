package app_test

import (
	"cmp"
	"encoding/json"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestAnAutomaticCommandIsPreparedOnceAndReplayedWithoutDuplicates(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, farFuture)
	service := newTestService(t)

	first := evaluateIntent(t, db, service, intentID, fixtureNow)
	if first.Result != "approved" || first.CommandID == "" {
		t.Fatalf("first result = %+v, want approved with a command", first)
	}
	commandDocument := storedCommand(t, db, first.CommandID)
	if err := contractsv1.Validate(contractsv1.SchemaCommand, commandDocument); err != nil {
		t.Fatalf("generated command must validate: %v", err)
	}
	if commandDocument["policy_digest"] == "" || commandDocument["not_before_mono_us"] != float64(0) {
		t.Fatalf("generated command missing device freshness or policy binding: %#v", commandDocument)
	}

	second := evaluateIntent(t, db, service, intentID, fixtureNow.Add(time.Second))
	if second.Result != "approved" || second.CommandID != first.CommandID {
		t.Fatalf("replay = %+v, want the same approved command %s", second, first.CommandID)
	}
	commands := scalar[int](t, db, "SELECT COUNT(*) FROM commands WHERE intent_id = ?", intentID)
	outbox := scalar[int](t, db, "SELECT COUNT(*) FROM outbox WHERE kind = 'command' AND aggregate_id = ?", first.CommandID)
	audits := scalar[int](t, db, "SELECT COUNT(*) FROM policy_evaluations WHERE intent_id = ?", intentID)
	if commands != 1 || outbox != 1 || audits != 2 {
		t.Fatalf("after a replay: commands=%d outbox=%d audits=%d, want 1/1/2", commands, outbox, audits)
	}
}

func storedCommand(t *testing.T, db *storage.DB, commandID string) map[string]any {
	t.Helper()
	var document map[string]any
	raw := scalar[[]byte](t, db, "SELECT command_json FROM commands WHERE command_id = ?", commandID)
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode generated command: %v", err)
	}
	return document
}

// Invariant 7: policy revalidates an intent against current state immediately
// before dispatch. Each case changes the basis the Decision was made on after
// the intent was proposed; none may yield a command.
func TestEvaluationRevalidatesTheIntentAgainstCurrentState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		risk       string
		current    int
		proposed   int
		material   int
		expiresAt  time.Time
		change     string
		wantResult string
		wantReason string
	}{
		{name: "a newer material Situation version makes the intent stale", risk: "R1", current: 2, proposed: 1, expiresAt: farFuture, wantResult: "stale", wantReason: "situation_version_stale"},
		{name: "a newer version that is not material keeps the intent fresh", risk: "R1", current: 3, proposed: 1, material: 1, expiresAt: farFuture, wantResult: "approved", wantReason: "automatic_r0_r1"},
		{name: "an expired intent is refused", risk: "R1", current: 1, proposed: 1, expiresAt: fixtureNow.Add(-time.Hour), wantResult: "expired", wantReason: "intent_expired"},
		{name: "a tripped interlock refuses a low-risk intent", risk: "R1", current: 1, proposed: 1, expiresAt: farFuture, change: tripInterlock, wantResult: "denied", wantReason: "interlock_not_ready"},
		{name: "an episode that produced no decision is refused", risk: "R1", current: 1, proposed: 1, expiresAt: farFuture, change: abandonEpisode, wantResult: "denied", wantReason: "episode_not_concluded"},
		{name: "provisional source health refuses a consequential intent", risk: "R2", current: 1, proposed: 1, expiresAt: farFuture, change: provisionalSourceHealth, wantResult: "denied", wantReason: "source_health_incomplete"},
		{name: "a consequential intent waits for a human", risk: "R2", current: 1, proposed: 1, expiresAt: farFuture, wantResult: "approval_required", wantReason: "risk_requires_approval"},
		{name: "a high-risk intent is denied", risk: "R3", current: 1, proposed: 1, expiresAt: farFuture, wantResult: "denied", wantReason: "risk_policy_denied"},
		{name: "a catalog approval requirement turns a low-risk intent into an approval request", risk: "R1", current: 1, proposed: 1, expiresAt: farFuture, change: requireCatalogApproval, wantResult: "approval_required", wantReason: "risk_requires_approval"},
		{name: "a catalog approval requirement never softens a high-risk denial", risk: "R4", current: 1, proposed: 1, expiresAt: farFuture, change: requireCatalogApproval, wantResult: "denied", wantReason: "risk_policy_denied"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			db, intentID := openPolicyFixture(t, test.risk, test.current, test.proposed, test.expiresAt)
			setMaterialVersion(t, db, cmp.Or(test.material, test.current))
			if test.change != "" {
				exec(t, db, test.change)
			}
			result := evaluateIntent(t, db, newTestService(t), intentID, fixtureNow)
			if result.Result != test.wantResult || result.Reason != test.wantReason {
				t.Fatalf("result = %s/%s, want %s/%s", result.Result, result.Reason, test.wantResult, test.wantReason)
			}
			if commands, outbox := commandAndOutboxCounts(t, db); (test.wantResult == "approved") != (commands == 1 && outbox == 1) || commands > 1 {
				t.Fatalf("result %s left commands=%d outbox=%d", test.wantResult, commands, outbox)
			}
		})
	}
}

func TestAnUnreadableIntentExpiryDeniesThatIntentAndLeavesTheQueue(t *testing.T) {
	t.Parallel()
	for name, corrupt := range map[string]string{"garbage": "not a time", "empty": "", "space separated": "2099-01-01 00:00:00"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, intentID := openPolicyFixture(t, "R1", 1, 1, farFuture)
			exec(t, db, "UPDATE intents SET expires_at = ? WHERE intent_id = ?", corrupt, intentID)

			result := evaluateIntent(t, db, newTestService(t), intentID, fixtureNow)
			if result.Result != "denied" || result.Reason != "intent_expiry_unreadable" {
				t.Fatalf("result = %+v, want denied/intent_expiry_unreadable", result)
			}
			if next, found, err := policy.NextPendingIntent(t.Context(), db.DB, "tenant"); err != nil || found {
				t.Fatalf("pending intent %q found=%v err=%v, want the queue to move on", next, found, err)
			}
		})
	}
}

func TestAnApprovedIntentThatRequiresApprovalDispatchesInsteadOfRequestingAgain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		risk  string
		setup string
	}{
		{name: "consequential intent", risk: "R2"},
		{name: "low-risk intent the catalog marks requires_approval", risk: "R1", setup: requireCatalogApproval + `;
			INSERT INTO approval_authorities (tenant_id, entity_id, risk_class, role_id) VALUES ('tenant', 'motor-1', 'R1', 'role-approver')`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			db, intentID := openPolicyFixture(t, test.risk, 1, 1, fixtureNow.Add(time.Hour))
			if test.setup != "" {
				exec(t, db, test.setup)
			}
			p := requestApproval(t, db, intentID)

			resolved := p.approve(t)
			if resolved.Result != "approved" || resolved.CommandID == "" || resolved.Reason != "approved_by_human" {
				t.Fatalf("resolved = %+v, want approved_by_human with a command", resolved)
			}
			if approvals := scalar[int](t, db, "SELECT COUNT(*) FROM approvals WHERE intent_id = ?", intentID); approvals != 1 {
				t.Fatalf("approvals = %d, want exactly 1 (no approve to re-pending loop)", approvals)
			}
			if notice := scalar[string](t, db, "SELECT event_type FROM notifications WHERE event_id = ?", "approval.requested:"+p.approvalID); notice != (notify.ApprovalRequested{}).EventType() {
				t.Fatalf("approval request announced as %q", notice)
			}
		})
	}
}

func TestAReEvaluationThatFindsAStoredCommandReportsAlreadyCommandedAndQueuesNothing(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, farFuture)
	service := newTestService(t)
	first := evaluateIntent(t, db, service, intentID, fixtureNow)
	if _, err := db.ExecContext(t.Context(), "UPDATE intents SET policy_status = 'pending' WHERE intent_id = ?", intentID); err != nil {
		t.Fatal(err)
	}
	again := evaluateIntent(t, db, service, intentID, fixtureNow.Add(time.Second))
	if again.Result != "approved" || again.Reason != "already_commanded" || again.CommandID != first.CommandID {
		t.Fatalf("re-evaluation = %+v, want approved/already_commanded with command %s", again, first.CommandID)
	}
	outbox := scalar[int](t, db, "SELECT COUNT(*) FROM outbox WHERE kind = 'command' AND aggregate_id = ?", first.CommandID)
	audits := scalar[int](t, db, "SELECT COUNT(*) FROM policy_evaluations WHERE intent_id = ?", intentID)
	if outbox != 1 || audits != 2 {
		t.Fatalf("outbox=%d audits=%d, want the one queued command and one audit per evaluation", outbox, audits)
	}
}

func TestARiskyIntentReEvaluatedWhileItsApprovalIsPendingReusesThatApproval(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, farFuture)
	service := newTestService(t)
	first := evaluateIntent(t, db, service, intentID, fixtureNow)
	if first.Result != "approval_required" || first.ApprovalID == "" {
		t.Fatalf("first evaluation = %+v, want an approval request", first)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE intents SET policy_status = 'pending' WHERE intent_id = ?", intentID); err != nil {
		t.Fatal(err)
	}
	again := evaluateIntent(t, db, service, intentID, fixtureNow.Add(time.Second))
	if again.ApprovalID != first.ApprovalID || again.Result != "approval_required" {
		t.Fatalf("re-evaluation = %+v, want the pending approval %s reused", again, first.ApprovalID)
	}
	if approvals := scalar[int](t, db, "SELECT COUNT(*) FROM approvals WHERE intent_id = ?", intentID); approvals != 1 {
		t.Fatalf("approvals = %d, want one", approvals)
	}
}

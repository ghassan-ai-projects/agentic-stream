package notify_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

func TestFacadeLifecycleConstructorsDeriveTheEventIdentity(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	trace := contractsv1.TraceContext{}
	digest := "sha256:" + strings.Repeat("1", 64)
	tests := []struct {
		name  string
		event notify.LifecycleEvent
		id    string
	}{
		{"approval requested", notify.ApprovalRequestedEvent("acme", notify.ApprovalRequested{ApprovalID: "appr_1", SituationID: "sit_1", IntentDigest: digest}, at, trace), "approval.requested:appr_1"},
		{"approval withdrawn", notify.ApprovalWithdrawnEvent("acme", notify.ApprovalWithdrawn{ApprovalID: "appr_1", SituationID: "sit_1"}, at, trace), "approval.withdrawn:appr_1"},
		{"approval resolved", notify.ApprovalResolvedEvent("acme", notify.ApprovalResolved{ApprovalID: "appr_1", SituationID: "sit_1", Status: "denied"}, at, trace), "approval.resolved:appr_1:denied"},
		{"command dispatched", notify.CommandDispatchedEvent("acme", notify.CommandDispatched{CommandID: "cmd_1", OutcomeID: "out_1"}, at, trace), "command.dispatched:cmd_1:out_1"},
		{"outcome recorded", notify.OutcomeRecordedEvent("acme", notify.OutcomeRecorded{CommandID: "cmd_1", OutcomeID: "out_1"}, at, trace), "outcome.recorded:out_1"},
		{"outcome reconciled", notify.OutcomeReconciledEvent("acme", notify.OutcomeReconciled{CommandID: "cmd_1", OutcomeID: "out_1"}, at, trace), "outcome.reconciled:out_1"},
		{"reconsideration admitted", notify.ReconsiderationAdmittedEvent("acme", notify.ReconsiderationAdmitted{ReconsiderationID: "rec_1", SituationID: "sit_1"}, at, trace), "reconsideration.admitted:rec_1"},
		{"situation superseded", notify.SituationSupersededEvent("acme", "sch_1", notify.SituationSuperseded{SituationID: "sit_1", SupersededVersion: 2, ReplacementVersion: 3}, at, trace), "situation.superseded:sit_1:2:3:sch_1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if test.event.ID != test.id || test.event.TenantID != "acme" || !test.event.At.Equal(at) {
				t.Fatalf("event = %+v, want id %q for tenant acme at %v", test.event, test.id, at)
			}
		})
	}
}

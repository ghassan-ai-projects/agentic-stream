package domain

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func TestLifecycleConstructorsFixEveryEventIdentity(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("1", 64)
	at := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	trace := contractsv1.TraceContext{}
	tests := []struct {
		name                   string
		event                  LifecycleEvent
		id, subject, partition string
	}{
		{"approval requested", ApprovalRequestedEvent("acme", ApprovalRequested{ApprovalID: "appr_1", IntentID: "int_1", DecisionID: "dec_1", SituationID: "sit_1", SituationVersion: 3, IntentDigest: digest, SnapshotDigest: digest, RiskClass: "R2", ExpiresAt: "2026-10-09T12:00:00Z", Audience: "relay", Summary: "s", Delta: map[string]any{}, Hypothesis: "h", Evidence: []string{"e"}, Action: map[string]any{}, DeclineConsequence: "d"}, at, trace), "approval.requested:appr_1", "approval/appr_1", "sit_1"},
		{"approval withdrawn", ApprovalWithdrawnEvent("acme", ApprovalWithdrawn{ApprovalID: "appr_1", IntentID: "int_1", SituationID: "sit_1", SituationVersion: 3, Reason: "situation_version_conflict"}, at, trace), "approval.withdrawn:appr_1", "approval/appr_1", "sit_1"},
		{"approval resolved", ApprovalResolvedEvent("acme", ApprovalResolved{ApprovalID: "appr_1", IntentID: "int_1", DecisionID: "dec_1", SituationID: "sit_1", SituationVersion: 3, Status: "denied", Reason: "r"}, at, trace), "approval.resolved:appr_1:denied", "approval/appr_1", "sit_1"},
		{"command dispatched", CommandDispatchedEvent("acme", CommandDispatched{CommandID: "cmd_1", IntentID: "int_1", OutcomeID: "out_1", Status: "succeeded"}, at, trace), "command.dispatched:cmd_1:out_1", "command/cmd_1", "cmd_1"},
		{"outcome recorded", OutcomeRecordedEvent("acme", OutcomeRecorded{IntentID: "int_1", CommandID: "cmd_1", OutcomeID: "out_1", OutcomeDigest: digest, Status: "succeeded", ReconciliationStatus: "observed"}, at, trace), "outcome.recorded:out_1", "outcome/out_1", "cmd_1"},
		{"outcome reconciled", OutcomeReconciledEvent("acme", OutcomeReconciled{IntentID: "int_1", CommandID: "cmd_1", OutcomeID: "out_1", OutcomeDigest: digest, FinalStatus: "succeeded", ReconciliationStatus: "reconciled", Verdict: "verified", ReconciliationVersion: 2}, at, trace), "outcome.reconciled:out_1", "outcome/out_1", "cmd_1"},
		{"situation superseded", SituationSupersededEvent("acme", "sch_1", SituationSuperseded{SituationID: "sit_1", SupersededVersion: 2, ReplacementVersion: 3, Reason: "newer_situation_version_admitted"}, at, trace), "situation.superseded:sit_1:2:3:sch_1", "situation/sit_1", "sit_1"},
		{"reconsideration admitted", ReconsiderationAdmittedEvent("acme", ReconsiderationAdmitted{ReconsiderationID: "rec_1", SituationID: "sit_1", SupersededVersion: 2, CorrectionVersion: 3, InvalidatedCommandID: "cmd_1", InvalidatedOutcomeID: "out_1", TriggerID: "trg_1", SchedulerItemID: "sch_1"}, at, trace), "reconsideration.admitted:rec_1", "situation/sit_1", "sit_1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			event := test.event
			if event.ID != test.id || event.Subject != test.subject || event.PartitionKey != test.partition || event.TenantID != "acme" || !event.At.Equal(at) {
				t.Fatalf("event = %+v, want id %q subject %q partition %q", event, test.id, test.subject, test.partition)
			}
			if _, err := NewLifecycleEvent(event); err != nil {
				t.Fatalf("constructed event violates the contract: %v", err)
			}
		})
	}
}

func TestEveryLifecycleTypeHasAConstructor(t *testing.T) {
	t.Parallel()
	constructed := []Payload{
		ApprovalRequestedEvent("t", ApprovalRequested{}, time.Time{}, contractsv1.TraceContext{}).Payload,
		ApprovalWithdrawnEvent("t", ApprovalWithdrawn{}, time.Time{}, contractsv1.TraceContext{}).Payload,
		ApprovalResolvedEvent("t", ApprovalResolved{}, time.Time{}, contractsv1.TraceContext{}).Payload,
		CommandDispatchedEvent("t", CommandDispatched{}, time.Time{}, contractsv1.TraceContext{}).Payload,
		OutcomeRecordedEvent("t", OutcomeRecorded{}, time.Time{}, contractsv1.TraceContext{}).Payload,
		OutcomeReconciledEvent("t", OutcomeReconciled{}, time.Time{}, contractsv1.TraceContext{}).Payload,
		SituationSupersededEvent("t", "i", SituationSuperseded{}, time.Time{}, contractsv1.TraceContext{}).Payload,
		ReconsiderationAdmittedEvent("t", ReconsiderationAdmitted{}, time.Time{}, contractsv1.TraceContext{}).Payload,
	}
	for _, payload := range constructed {
		if !slices.Contains(lifecycleTypes, payload.EventType()) {
			t.Fatalf("constructor payload type %q is not a lifecycle type", payload.EventType())
		}
	}
	if len(constructed) != len(lifecycleTypes) {
		t.Fatalf("constructors = %d, lifecycle types = %d", len(constructed), len(lifecycleTypes))
	}
}

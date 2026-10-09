package notify_test

import (
	"database/sql"
	"fmt"
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

func TestLifecycleEventsUseStableTypesAndDurableCursors(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	payloads := lifecyclePayloads()
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		for i, payload := range payloads {
			if err := notify.AppendLifecycleEvent(ctx, tx, lifecycleRequest(i, payload)); err != nil {
				return fmt.Errorf("append lifecycle event: %w", err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	page, err := outbox.ReadPage(ctx, notify.PageRequest{TenantID: "acme", Limit: 10}, baseTime)
	if err != nil || len(page.Records) != len(payloads) || page.NextCursor != int64(len(payloads)) {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	for i, record := range page.Records {
		data := record.Event.Data.(map[string]any)
		if record.Cursor != int64(i+1) || record.Event.Type != payloads[i].EventType() || record.Event.Source != notify.SourceForTenant("acme") || data["tenant_id"] != "acme" || data["source_authority"] != record.Event.Source {
			t.Fatalf("record[%d]=%+v", i, record)
		}
	}
}

func TestLifecycleEventRefusesAContractViolation(t *testing.T) {
	t.Parallel()
	db, outbox := openOutbox(t)
	request := lifecycleRequest(0, notify.OutcomeRecorded{IntentID: "int_1", Status: "bogus"})
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return notify.AppendLifecycleEvent(t.Context(), tx, request) })
	if err == nil {
		t.Fatal("event violating the contract was appended")
	}
	if page, _ := outbox.ReadPage(t.Context(), notify.PageRequest{TenantID: "acme", Limit: 10}, baseTime); len(page.Records) != 0 {
		t.Fatalf("refused event was stored: %+v", page)
	}
}

func lifecycleRequest(index int, payload notify.Payload) notify.LifecycleEvent {
	return notify.LifecycleEvent{
		ID: fmt.Sprintf("lifecycle-%d", index), TenantID: "acme", Subject: "subject/1", PartitionKey: "partition-1",
		Payload: payload, At: baseTime.Add(time.Duration(index) * time.Second),
	}
}

// lifecyclePayloads is one valid payload per stable lifecycle event type.
func lifecyclePayloads() []notify.Payload {
	digest := "sha256:" + strings.Repeat("1", 64)
	return []notify.Payload{
		notify.OutcomeRecorded{IntentID: "int_1", CommandID: "cmd_1", OutcomeID: "out_1", OutcomeDigest: digest, Status: "succeeded", ReconciliationStatus: "observed"},
		notify.OutcomeReconciled{IntentID: "int_1", CommandID: "cmd_1", OutcomeID: "out_1", OutcomeDigest: digest, FinalStatus: "succeeded", ReconciliationStatus: "reconciled", Verdict: "verified", ReconciliationVersion: 2},
		notify.ApprovalRequested{ApprovalID: "appr_1", IntentID: "int_1", DecisionID: "dec_1", SituationID: "sit_1", SituationVersion: 3, IntentDigest: digest, SnapshotDigest: digest, RiskClass: "R2", ExpiresAt: "2026-08-14T13:00:00Z", Audience: "stream-approval-relay", Summary: "Approval is required", Delta: map[string]any{"phase": "warning"}, Hypothesis: "The motor is degrading", Evidence: []string{"evt_1"}, Action: map[string]any{"target": "motor_1"}, DeclineConsequence: "The intent will not be dispatched."},
		notify.ApprovalWithdrawn{ApprovalID: "appr_1", IntentID: "int_1", SituationID: "sit_1", SituationVersion: 3, Reason: "situation_version_conflict"},
		notify.ApprovalResolved{ApprovalID: "appr_1", IntentID: "int_1", DecisionID: "dec_1", SituationID: "sit_1", SituationVersion: 3, Status: "approved", Reason: "approved"},
		notify.CommandDispatched{CommandID: "cmd_1", IntentID: "int_1", OutcomeID: "out_1", Status: "succeeded"},
		notify.SituationSuperseded{SituationID: "sit_1", SupersededVersion: 2, ReplacementVersion: 3, Reason: "newer_situation_version_admitted"},
		notify.ReconsiderationAdmitted{ReconsiderationID: "rec_1", SituationID: "sit_1", SupersededVersion: 2, CorrectionVersion: 3, InvalidatedCommandID: "cmd_1", InvalidatedOutcomeID: "out_1", TriggerID: "trg_1", SchedulerItemID: "sch_1"},
	}
}

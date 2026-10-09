package store

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestEvaluationEventPinsItsIdentityAndInstantText(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	event := evaluationEvent(domain.Evaluation{TriggerID: "trg-1", SituationID: "sit-1", SituationVersion: 2, Outcome: "admitted", EvaluatedAt: at}, "tenant")
	if want := "trg-1:admitted:2026-08-12T12:00:00.000000000Z"; event.ID != want {
		t.Fatalf("event id = %q, want %q: the id is the notification dedupe key", event.ID, want)
	}
	if !event.Time.Equal(at) {
		t.Fatalf("event time = %v", event.Time)
	}
}

func TestSupersededWithdrawalEventUsesTheSharedNotifyIdentity(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	withdrawal := approvalledger.Withdrawal{ApprovalID: "appr_1", IntentID: "int_1", SituationID: "sit_1", SituationVersion: 3, Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"}
	got := supersededWithdrawalEvent(withdrawal, "acme", sources.NewVirtual(now))
	payload := notify.ApprovalWithdrawn{ApprovalID: "appr_1", IntentID: "int_1", SituationID: "sit_1", SituationVersion: 3, Reason: "situation_version_conflict"}
	want := notify.ApprovalWithdrawnEvent("acme", payload, now, contractsv1.TraceContext{Traceparent: withdrawal.Traceparent})
	if got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
}

func TestSupersededItemEventUsesTheSharedNotifyIdentity(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	replacement := ReplacementVersion{SituationID: "sit_1", TenantID: "acme", Version: 4, Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"}
	got := supersededItemEvent(replacement, SupersededItem{ID: "sch_1", Version: 3}, now)
	payload := notify.SituationSuperseded{SituationID: "sit_1", SupersededVersion: 3, ReplacementVersion: 4, Reason: "newer_situation_version_admitted"}
	want := notify.SituationSupersededEvent("acme", "sch_1", payload, now, contractsv1.TraceContext{Traceparent: replacement.Traceparent})
	if got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
}

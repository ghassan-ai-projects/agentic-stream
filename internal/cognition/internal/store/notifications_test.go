package store

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

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

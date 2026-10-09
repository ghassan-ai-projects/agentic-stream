package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
)

// fencedApprovalHTTP wires the pipeline to a real runtime owner lease and
// decision-epoch control, with the approval's episode bound to that epoch.
func fencedApprovalHTTP(t *testing.T) (approvalHTTPFixture, *control.EpochControl) {
	t.Helper()
	var epochControl *control.EpochControl
	f := openApprovalHTTP(t, func(cfg *runtime.PipelineConfig) {
		owner := &control.RuntimeOwner{DB: cfg.DB, InstanceID: "approvals", Lease: time.Minute, Now: cfg.Clock.Now}
		if err := owner.Claim(t.Context(), "approval-epoch"); err != nil {
			t.Fatal(err)
		}
		epochControl = &control.EpochControl{DB: cfg.DB, Now: cfg.Clock.Now}
		cfg.Owner, cfg.OwnerEpoch, cfg.EpochControl = owner, "approval-epoch", epochControl
		exec(t, cfg.DB, "UPDATE episodes SET policy_epoch = 'approval-epoch'")
	})
	return f, epochControl
}

func TestHTTPResolutionOfAKilledDecisionEpochDeniesTheIntent(t *testing.T) {
	t.Parallel()
	f, epochControl := fencedApprovalHTTP(t)
	body := signedApproval(t, f, true)
	if err := epochControl.Kill(t.Context(), "approval-epoch"); err != nil {
		t.Fatal(err)
	}

	rec := approvalRequest(t, f.handler, http.MethodPost, "/v1/approvals/"+f.approvalID, body)
	commands, outbox := commandAndOutboxCounts(t, f.db)
	status := scalar[string](t, f.db, "SELECT policy_status FROM intents WHERE intent_id = ?", f.intentID)
	if rec.Code != http.StatusOK || commands != 0 || outbox != 0 || status != "denied" {
		t.Fatalf("POST = %d %s, commands=%d outbox=%d intent %q; want the intent denied with nothing dispatched", rec.Code, rec.Body.String(), commands, outbox, status)
	}
}

func TestHTTPResolutionAfterTheRuntimeOwnerLeaseLapsedIsRefusedAndConsumesNothing(t *testing.T) {
	t.Parallel()
	f, _ := fencedApprovalHTTP(t)
	body := signedApproval(t, f, true)
	f.clock.Advance(2 * time.Minute)

	rec := approvalRequest(t, f.handler, http.MethodPost, "/v1/approvals/"+f.approvalID, body)
	commands, outbox := commandAndOutboxCounts(t, f.db)
	if rec.Code != http.StatusServiceUnavailable || f.status(t) != "pending" || commands != 0 || outbox != 0 {
		t.Fatalf("POST = %d %s, status %q commands=%d outbox=%d; want 503 with the approval still pending", rec.Code, rec.Body.String(), f.status(t), commands, outbox)
	}
}

package app_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
)

func TestHTTPResolutionFencesKilledEpochAndLostOwner(t *testing.T) {
	for _, failure := range []string{"epoch killed", "owner lost"} {
		t.Run(failure, func(t *testing.T) {
			var owner *control.RuntimeOwner
			var epochControl *control.EpochControl
			f := openApprovalHTTP(t, func(cfg *runtime.PipelineConfig) {
				owner = &control.RuntimeOwner{DB: cfg.DB, InstanceID: "approvals", Lease: time.Minute, Now: cfg.Clock.Now}
				if err := owner.Claim(t.Context(), "approval-epoch"); err != nil {
					t.Fatal(err)
				}
				epochControl = &control.EpochControl{DB: cfg.DB, Now: cfg.Clock.Now}
				cfg.Owner, cfg.OwnerEpoch, cfg.EpochControl = owner, "approval-epoch", epochControl
				if _, err := cfg.DB.ExecContext(t.Context(), "UPDATE episodes SET policy_epoch='approval-epoch'"); err != nil {
					t.Fatal(err)
				}
			})
			body := signedApproval(t, f, true)
			expected := 200
			if failure == "epoch killed" {
				if err := epochControl.Kill(t.Context(), "approval-epoch"); err != nil {
					t.Fatal(err)
				}
			} else {
				f.clock.Advance(2 * time.Minute)
				expected = 503
			}
			rec := approvalRequest(t, f.handler, "POST", "/v1/approvals/"+f.id, body)
			commands, outbox := approvalCounts(t, f.db)
			if rec.Code != expected || commands != 0 || outbox != 0 {
				t.Fatal(rec.Code, rec.Body.String(), commands, outbox)
			}
			if failure == "owner lost" && approvalStatus(t, f) != "pending" {
				t.Fatal("lost owner consumed approval")
			}
			if failure == "epoch killed" {
				var status string
				if err := f.db.QueryRowContext(t.Context(), "SELECT policy_status FROM intents WHERE intent_id='int-policy'").Scan(&status); err != nil {
					t.Fatal(err)
				}
				if status != "denied" {
					t.Fatal("killed epoch intent", status)
				}
			}
		})
	}
}

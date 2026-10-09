package app_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestDispatchPolicyDecidesWhetherAnExecutorIsAdmittedAndWhetherItsDecisionsReachActions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, executor, policy string
		demo                   bool
		wantAdmitted           int
		wantDispatched         int
	}{
		{"fixture is refused on a production route under active policy", "fixture", "active", false, 0, 0},
		{"fixture is refused on a production route under shadow policy", "fixture", "shadow", false, 0, 0},
		{"fixture is admitted in demo mode and stays shadow", "fixture", "shadow", true, 1, 0},
		{"native executor under active policy dispatches", "native", "active", false, 1, 1},
		{"native executor under shadow policy is scored, not dispatched", "native", "shadow", false, 1, 0},
		{"tamoz executor under active policy dispatches", "tamoz", "active", false, 1, 1},
		{"tamoz executor under shadow policy is scored, not dispatched", "tamoz", "shadow", false, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			epoch := "epoch-" + tt.executor + "-" + tt.policy
			pipeline, db := openPipeline(t, thingSpec(tt.executor, tt.policy), runtime.PipelineConfig{OwnerEpoch: epoch, DemoMode: tt.demo})
			report, err := pipeline.RunJSONL(t.Context(), highLevelTrace(t))
			if err != nil {
				t.Fatalf("a refused executor must be quarantined, not fail the batch: %v", err)
			}
			if report.EpisodesAdmitted != tt.wantAdmitted || report.CommandsDispatched != tt.wantDispatched {
				t.Fatalf("report = %+v, want %d admitted and %d dispatched", report, tt.wantAdmitted, tt.wantDispatched)
			}
			assertAdmissionRecord(t, db, tt.policy, epoch, tt.wantAdmitted == 1)
			if tt.wantAdmitted == 1 && tt.policy == "shadow" {
				assertShadowWithoutEffects(t, db)
			}
		})
	}
}

func assertAdmissionRecord(t *testing.T, db *storage.DB, policy, epoch string, admitted bool) {
	t.Helper()
	if !admitted {
		if episodes, status := countRows(t, db, "episodes"), scalar[string](t, db, "SELECT status FROM scheduler_items"); episodes != 0 || status != "coalesced" {
			t.Fatalf("refused item: %d episodes, scheduler status %q; want 0 and coalesced (the queue keeps draining)", episodes, status)
		}
		return
	}
	if got := scalar[string](t, db, "SELECT dispatch_policy FROM episodes"); got != policy {
		t.Fatalf("recorded dispatch_policy = %q, want %q", got, policy)
	}
	if got := scalar[string](t, db, "SELECT policy_epoch FROM episodes"); got != epoch {
		t.Fatalf("recorded policy_epoch = %q, want %q", got, epoch)
	}
}

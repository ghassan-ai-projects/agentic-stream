package app_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestAdmitPendingAdmitsOrSkipsTheDueItemAccordingToTheScenario(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		given        scenario
		wantAdmitted int
		wantStatus   string
		wantReason   string
	}{
		{"a native item is admitted and stamped with the owner epoch", scenario{executor: "native"}, 1, "admitted", ""},
		{"a fixture on a production route is quarantined", scenario{executor: "fixture"}, 0, "coalesced", ""},
		{"a fixture is admitted in demo mode", scenario{executor: "fixture", demo: true}, 1, "admitted", ""},
		{"nothing is admitted while the epoch drains", scenario{executor: "native", drain: true}, 0, "pending", ""},
		{"a cost-rejected item is skipped", scenario{executor: "native", costKill: true}, 0, "coalesced", "cost ceiling or kill switch rejected"},
		{"an item past its expiry is expired", scenario{executor: "native", stale: true}, 0, "expired", "scheduler item expired: expired before admission"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db, admitter := pendingItem(t, tt.given)
			if admitted, err := admitter.AdmitPending(t.Context()); err != nil || admitted != tt.wantAdmitted {
				t.Fatalf("AdmitPending = %d, %v; want %d", admitted, err, tt.wantAdmitted)
			}
			if status := scalar[string](t, db, "SELECT status FROM scheduler_items"); status != tt.wantStatus {
				t.Fatalf("scheduler item status = %q, want %q", status, tt.wantStatus)
			}
			if reasons := evaluationReasons(t, db); !strings.Contains(reasons, tt.wantReason) {
				t.Fatalf("trigger evaluation reasons = %s, want %q", reasons, tt.wantReason)
			}
			if tt.wantAdmitted == 1 {
				if epoch := scalar[string](t, db, "SELECT policy_epoch FROM episodes"); epoch != ownerEpoch {
					t.Fatalf("policy_epoch = %q, want %q", epoch, ownerEpoch)
				}
			}
			if again, err := admitter.AdmitPending(t.Context()); err != nil || again != 0 {
				t.Fatalf("second AdmitPending = %d, %v; a handled item must not be admitted twice", again, err)
			}
		})
	}
}

func TestExpiredItemsNoLongerFillGlobalCapacityOrTheNextPoll(t *testing.T) {
	t.Parallel()
	db, admitter := pendingItem(t, scenario{executor: "native", stale: true})
	addPendingItems(t, db, 100, "2026-01-01T00:00:00.000000000Z")
	if admitted, err := admitter.AdmitPending(t.Context()); err != nil || admitted != 0 {
		t.Fatalf("AdmitPending = %d, %v", admitted, err)
	}
	if counts := itemCounts(t, db); counts["pending"] != 0 || counts["expired"] != 101 {
		t.Fatalf("item statuses after admission = %v, want 101 expired and none pending", counts)
	}
}

func TestAnUnreadableSchedulerTimeExpiresThatItemAndAdmissionContinues(t *testing.T) {
	t.Parallel()
	db, admitter := pendingItem(t, scenario{executor: "native"})
	addPendingItems(t, db, 1, "not a time")
	if admitted, err := admitter.AdmitPending(t.Context()); err != nil || admitted != 1 {
		t.Fatalf("AdmitPending = %d, %v; want the readable item admitted", admitted, err)
	}
	if counts := itemCounts(t, db); counts["admitted"] != 1 || counts["expired"] != 1 || counts["pending"] != 0 {
		t.Fatalf("item statuses = %v", counts)
	}
	if reasons := evaluationReasons(t, db); !strings.Contains(reasons, "scheduler item expired: unreadable expires_at") {
		t.Fatalf("trigger evaluation reasons = %s", reasons)
	}
}

func TestAdmitterRequiresItsStoreAssemblerAndClock(t *testing.T) {
	t.Parallel()
	for name, cfg := range map[string]app.AdmitterConfig{
		"empty":        {},
		"no assembler": {Store: &store.PipelineStore{}, Clock: sources.Physical()},
		"no clock":     {Store: &store.PipelineStore{}},
	} {
		if admitter, err := app.NewAdmitter(cfg); err == nil || admitter != nil {
			t.Errorf("%s: construction = (%v, %v)", name, admitter, err)
		}
	}
}

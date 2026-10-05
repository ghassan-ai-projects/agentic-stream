package authority_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
)

func TestSafeStopLatchesTheBootOnThePriorityPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if latched, err := f.service.SafeStopLatched(t.Context(), bootA); err != nil || latched {
		t.Fatalf("latched before any safe stop: %v, %v", latched, err)
	}
	f.clock.Advance(runtimeLease)
	if err := f.service.RecordSafeStop(t.Context(), fanClaim, authority.SafeStopRequested, map[string]any{"reason": "operator"}); err != nil {
		t.Fatalf("safe stop after authority loss: %v", err)
	}
	if latched, err := f.service.SafeStopLatched(t.Context(), bootA); err != nil || !latched {
		t.Fatalf("safe stop did not latch: %v, %v", latched, err)
	}
	if latched, err := f.service.SafeStopLatched(t.Context(), bootB); err != nil || latched {
		t.Fatalf("latch leaked to a new boot: %v, %v", latched, err)
	}
	if err := f.service.RecordSafeStop(t.Context(), fanClaim, "safe_stop_cleared", nil); err == nil {
		t.Fatal("unknown safe-stop stage was accepted")
	}
	if _, err := f.service.SafeStopLatched(t.Context(), authority.DeviceBoot{DeviceID: "d"}); err == nil {
		t.Fatal("partial device boot was accepted")
	}
}

func TestRecordSafetyEventValidatesAndStores(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.service.RecordSafetyEvent(t.Context(), authority.SafetyEvent{Type: "unknown", Target: "fan-01"}); err == nil {
		t.Fatal("unknown safety event type was accepted")
	}
	if err := f.service.RecordSafetyEvent(t.Context(), authority.SafetyEvent{Type: "physical_transition", Target: "fan-01", Details: map[string]any{
		"evidence_complete": true, "source": "independent-feedback",
		"evidence_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	}}); err != nil {
		t.Fatal(err)
	}
	if f.count(t, `SELECT COUNT(*) FROM device_safety_events WHERE occurred_at = '2026-10-05T12:00:00.000000000Z'`) != 1 {
		t.Fatal("safety event was not stored at the service clock time")
	}
	if !authority.PhysicalEvidenceComplete(map[string]any{"evidence_complete": true, "source": "s",
		"evidence_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"}) {
		t.Fatal("complete physical evidence was rejected")
	}
}

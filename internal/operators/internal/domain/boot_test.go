package domain_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/operators/internal/domain"
)

func bootTemperature(celsius float64, boot string, sequence uint64) map[string]any {
	return map[string]any{"celsius": celsius, "quality": "valid", "boot_id": boot, "seq": float64(sequence)}
}

func windowOfBoot(h *harness, boot string) *domain.WindowState {
	for _, blob := range h.ps.OperatorStates["op1"] {
		if blob != nil && blob.Window != nil && blob.Window.BootID == boot {
			return blob.Window
		}
	}
	return nil
}

func TestSequenceWrapWithinOneBootStaysInTheWindow(t *testing.T) {
	t.Parallel()
	h := newHarness(t, aggregateSpec("max"))
	h.applyOnTime(envelope("boot-a-high", "sensor.temperature", base, bootTemperature(100, "boot-A", 4294967295)))
	features := h.applyOnTime(envelope("boot-a-wrap", "sensor.temperature", base.Add(time.Minute), bootTemperature(200, "boot-A", 0)))
	if len(features) != 1 || features[0].Value != 200.0 || !slices.Equal(features[0].InputEventIDs, []string{"boot-a-high", "boot-a-wrap"}) {
		t.Fatalf("features = %+v, want max 200 over both samples", features)
	}
	if windowOfBoot(h, "boot-A") == nil {
		t.Fatal("boot-A window state was not persisted")
	}
}

func TestNewBootStartsAFreshWindowAndDiscardsTheOldBootsState(t *testing.T) {
	t.Parallel()
	h := newHarness(t, aggregateSpec("max"))
	h.applyOnTime(envelope("boot-a", "sensor.temperature", base, bootTemperature(100, "boot-A", 7)))
	features := h.applyOnTime(envelope("boot-b", "sensor.temperature", base.Add(time.Minute), bootTemperature(3, "boot-B", 0)))
	if len(features) != 1 || features[0].Value != 3.0 || !slices.Equal(features[0].InputEventIDs, []string{"boot-b"}) {
		t.Fatalf("features = %+v, want max 3 over boot-b only", features)
	}
	if windowOfBoot(h, "boot-A") != nil || windowOfBoot(h, "boot-B") == nil {
		t.Fatalf("window states = %+v, want only boot-B retained", h.ps.OperatorStates["op1"])
	}
}

func TestStaleBootCannotMutateTheActiveWindow(t *testing.T) {
	t.Parallel()
	h := newHarness(t, meanSpec())
	h.applyOnTime(envelope("boot-a", "sensor.temperature", base, bootTemperature(10, "boot-A", 0)))
	h.applyOnTime(envelope("boot-b", "sensor.temperature", base.Add(time.Minute), bootTemperature(20, "boot-B", 0)))
	stale := envelope("stale-a", "sensor.temperature", base.Add(2*time.Minute), bootTemperature(999, "boot-A", 1))
	if features := h.applyOnTime(stale); len(features) != 0 {
		t.Fatalf("stale boot emitted %+v, want nothing", features)
	}
	current := windowOfBoot(h, "boot-B")
	if current == nil || len(current.Samples) != 1 || current.Samples[0].EventID != "boot-b" {
		t.Fatalf("active window = %+v, want only boot-b", current)
	}
}

func TestBootlessEventsAreAdmittedOnlyUntilTheFirstIdentifiedBoot(t *testing.T) {
	t.Parallel()
	h := newHarness(t, meanSpec())
	if got := len(h.applyOnTime(temperature("legacy-1", base, 1))); got != 1 {
		t.Fatalf("bootless event before any boot: %d features, want 1", got)
	}
	if got := len(h.applyOnTime(envelope("boot", "sensor.temperature", base.Add(time.Minute), bootTemperature(2, "boot-A", 0)))); got != 1 {
		t.Fatalf("first identified boot: %d features, want 1", got)
	}
	if got := len(h.applyOnTime(temperature("legacy-2", base.Add(2*time.Minute), 3))); got != 0 {
		t.Fatalf("bootless event after an identified boot: %d features, want 0", got)
	}
}

func TestRetiredBootIsNeverAdmittedAgain(t *testing.T) {
	t.Parallel()
	h := newHarness(t, meanSpec())
	for i, boot := range []string{"boot-A", "boot-B", "boot-A"} {
		at := base.Add(time.Duration(i) * time.Minute)
		features := h.applyOnTime(envelope(fmt.Sprintf("e%d", i), "sensor.temperature", at, bootTemperature(1, boot, 0)))
		if wantEmitted := i < 2; (len(features) == 1) != wantEmitted {
			t.Fatalf("event %d from %s emitted %d features, want emitted=%v", i, boot, len(features), wantEmitted)
		}
	}
}

func TestBootHistoryIsBoundedAndFailsClosedWhenFull(t *testing.T) {
	t.Parallel()
	const maxSeenBoots = 64
	h := newHarness(t, meanSpec())
	for i := range maxSeenBoots {
		at := base.Add(time.Duration(i) * time.Minute)
		boot := fmt.Sprintf("boot-%d", i)
		if len(h.applyOnTime(envelope("e", "sensor.temperature", at, bootTemperature(1, boot, 0)))) != 1 {
			t.Fatalf("boot %d was refused before the history was full", i)
		}
	}
	at := base.Add(maxSeenBoots * time.Minute)
	if features := h.applyOnTime(envelope("overflow", "sensor.temperature", at, bootTemperature(1, "boot-new", 0))); len(features) != 0 {
		t.Fatalf("a new boot was admitted with a full history: %+v", features)
	}
}

func TestHeartbeatTimerSkipsThePreviousBootsState(t *testing.T) {
	t.Parallel()
	h := newHarness(t, heartbeatSpec())
	h.applyOnTime(heartbeat("hb-a", base, map[string]any{"boot_id": "boot-A"}))
	h.applyOnTime(heartbeat("hb-b", base.Add(time.Minute), map[string]any{"boot_id": "boot-B"}))
	features := h.fireTimer(base.Add(6 * time.Minute))
	if len(features) != 1 || features[0].BootID != "boot-B" {
		t.Fatalf("timer features = %+v, want only the active boot-B feature", features)
	}
}

func TestHeartbeatTimerSkipsBootlessStateOnceABootIsAdmitted(t *testing.T) {
	t.Parallel()
	h := newHarness(t, heartbeatSpec())
	h.applyOnTime(heartbeat("hb-legacy", base, nil))
	h.applyOnTime(heartbeat("hb-identified", base.Add(time.Minute), map[string]any{"boot_id": "boot-A"}))
	features := h.fireTimer(base.Add(6 * time.Minute))
	if len(features) != 1 || features[0].BootID != "boot-A" {
		t.Fatalf("timer features = %+v, want only the identified boot-A feature", features)
	}
}

func TestHeartbeatTimerFeatureCarriesTheBootScopedStateKey(t *testing.T) {
	t.Parallel()
	h := newHarness(t, heartbeatSpec())
	h.applyOnTime(heartbeat("hb", base, map[string]any{"boot_id": "boot-A"}))
	features := h.fireTimer(base.Add(5 * time.Minute))
	if len(features) != 1 || features[0].StateKey != "motor-17\x1fboot-A" || features[0].BootID != "boot-A" || features[0].EntityID != "motor-17" {
		t.Fatalf("timer features = %+v, want state key motor-17\\x1fboot-A on entity motor-17", features)
	}
}

func TestTimerStateIsActiveOnlyForTheCurrentBoot(t *testing.T) {
	t.Parallel()
	h := newHarness(t, heartbeatSpec())
	h.applyOnTime(heartbeat("hb-a", base, map[string]any{"boot_id": "boot-A"}))
	h.applyOnTime(heartbeat("hb-b", base.Add(time.Minute), map[string]any{"boot_id": "boot-B"}))
	tests := []struct {
		name     string
		ps       *domain.PartitionState
		stateKey string
		want     bool
	}{
		{"current boot", h.ps, "motor-17\x1fboot-B", true},
		{"previous boot", h.ps, "motor-17\x1fboot-A", false},
		{"bootless key after a boot was admitted", h.ps, "motor-17", false},
		{"entity never seen", h.ps, "motor-99", true},
		{"no partition state", nil, "motor-17\x1fboot-B", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := h.rt.IsTimerStateActive(tc.ps, tc.stateKey); got != tc.want {
				t.Fatalf("IsTimerStateActive(%q) = %v, want %v", tc.stateKey, got, tc.want)
			}
		})
	}
}

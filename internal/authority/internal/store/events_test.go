package store

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
)

func TestTheSafeStopLatchIsPerBootAndOnlyForSafeStopStages(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	work(t, s, func(tx *Tx) error { return tx.AppendAuthorityEvent(t.Context(), domain.ReleaseEvent(claim, testNow)) })
	if latched, err := s.SafeStopLatched(t.Context(), bootA); err != nil || latched {
		t.Fatalf("latched without a safe stop = %v, %v", latched, err)
	}
	work(t, s, func(tx *Tx) error {
		return tx.AppendAuthorityEvent(t.Context(), domain.SafeStopEvent(claim, domain.SafeStopFailed, nil, testNow))
	})
	if latched, err := s.SafeStopLatched(t.Context(), bootA); err != nil || !latched {
		t.Fatalf("safe stop did not latch = %v, %v", latched, err)
	}
	otherBoot := domain.DeviceBoot{DeviceID: bootA.DeviceID, BootID: "boot-B"}
	if latched, err := s.SafeStopLatched(t.Context(), otherBoot); err != nil || latched {
		t.Fatalf("latch leaked to a new boot = %v, %v", latched, err)
	}
}

func TestASafetyEventMayOrMayNotNameACommand(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	for _, commandID := range []string{"", "cmd-1"} {
		event := domain.SafetyEvent{Type: domain.SafetyUnsafeOutput, Target: "fan-01", CommandID: commandID, Details: map[string]any{}, Occurred: testNow}
		work(t, s, func(tx *Tx) error { return tx.AppendSafetyEvent(t.Context(), event) })
	}
	var withCommand, total int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(command_id), COUNT(*) FROM device_safety_events`).Scan(&withCommand, &total); err != nil {
		t.Fatal(err)
	}
	if withCommand != 1 || total != 2 {
		t.Fatalf("safety events with command=%d total=%d", withCommand, total)
	}
}

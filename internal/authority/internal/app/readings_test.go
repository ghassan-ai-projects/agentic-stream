package app_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
)

func (f *fixture) readingsOf(state map[string]any, device domain.DeviceBoot, receivedAt time.Time) domain.ReadingEvidence {
	reading := domain.Reading{Output: "level_percent", Value: 45, Good: true, BootID: device.BootID, ReceivedAt: receivedAt}
	return domain.ReadingEvidence{Device: device, Owner: ownerOne, Source: "plantwire.reading", State: state,
		Outputs: []string{"level_percent"}, Readings: []domain.Reading{reading}}
}

func (f *fixture) assertRequired(t *testing.T, device domain.DeviceBoot, want bool) {
	t.Helper()
	if required, err := f.service.ReconciliationRequired(t.Context(), device.DeviceID); err != nil || required != want {
		t.Fatalf("reconciliation required = %v, %v; want %v", required, err, want)
	}
}

func TestAReadingOnTheNewBootClearsTheReconciliationARebootOpened(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.recordState(t, bootA)
	stale := f.clock.Now()
	f.clock.Advance(time.Minute)
	state, required := f.recordState(t, bootB)
	if !required {
		t.Fatal("the reboot opened no reconciliation")
	}
	if cleared, err := f.service.ResolveWithReadings(t.Context(), f.readingsOf(state, bootA, f.clock.Now())); err != nil || cleared {
		t.Fatalf("a reading of the previous boot = %v, %v; want it not cleared", cleared, err)
	}
	f.assertRequired(t, bootB, true)
	if cleared, err := f.service.ResolveWithReadings(t.Context(), f.readingsOf(state, bootB, stale)); err != nil || !cleared {
		t.Fatalf("the first good reading of the new boot = %v, %v; want it cleared", cleared, err)
	}
	f.assertRequired(t, bootB, false)
	if recorded := f.count(t, `SELECT COUNT(*) FROM device_reconciliation WHERE device_id = ? AND last_resolution_status = 'succeeded'
		AND json_extract(resolution_evidence_json, '$.feedback.readings[0].output') = 'level_percent'`, bootB.DeviceID); recorded != 1 {
		t.Fatal("the resolution does not record the reading it used as evidence")
	}
}

func TestOnlyAReadingNewerThanAnUnknownOutcomeClearsItsReconciliation(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	state, _ := f.recordState(t, bootA)
	before := f.clock.Now()
	f.clock.Advance(time.Second)
	if err := f.service.OpenReconciliation(t.Context(), domain.ReconciliationOpening{Device: bootA, Owner: ownerOne, Reason: "outcome unknown"}); err != nil {
		t.Fatal(err)
	}
	opened := f.clock.Now()
	for _, receivedAt := range []time.Time{before, opened} {
		if cleared, err := f.service.ResolveWithReadings(t.Context(), f.readingsOf(state, bootA, receivedAt)); err != nil || cleared {
			t.Fatalf("a reading received at %s = %v, %v; want it not cleared", receivedAt, cleared, err)
		}
	}
	f.assertRequired(t, bootA, true)
	if cleared, err := f.service.ResolveWithReadings(t.Context(), f.readingsOf(state, bootA, opened.Add(time.Millisecond))); err != nil || !cleared {
		t.Fatalf("a reading after the opening = %v, %v; want it cleared", cleared, err)
	}
	f.assertRequired(t, bootA, false)
}

func TestReadingsNeverClearAReconciliationWhileACommandOfTheBootIsUnresolved(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	state, _ := f.recordState(t, bootA)
	f.insertUnknownOutcome(t, "cmd-unknown", bootA)
	if err := f.service.OpenReconciliation(t.Context(), domain.ReconciliationOpening{Device: bootA, Owner: ownerOne, Reason: "outcome unknown"}); err != nil {
		t.Fatal(err)
	}
	_, err := f.service.ResolveWithReadings(t.Context(), f.readingsOf(state, bootA, f.clock.Now().Add(time.Second)))
	if err == nil || !strings.Contains(err.Error(), "still require dispatcher reconciliation") {
		t.Fatalf("ResolveWithReadings with an unresolved command = %v, want the refusal", err)
	}
	f.assertRequired(t, bootA, true)
}

func TestAnUnresolvedNoReadbackCommandDoesNotHoldTheBarrierButAReadableOneStillDoes(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	state, _ := f.recordState(t, bootA)
	f.insertUnresolvedCommand(t, domain.CommandBinding{CommandID: "cmd-booking", Target: "led-b", Device: bootA, Owner: ownerOne, NoReadback: true})
	if err := f.service.OpenReconciliation(t.Context(), domain.ReconciliationOpening{Device: bootA, Owner: ownerOne, Reason: "outcome unknown"}); err != nil {
		t.Fatal(err)
	}
	f.insertUnknownOutcome(t, "cmd-readable", bootA)
	evidence := f.readingsOf(state, bootA, f.clock.Now().Add(time.Second))
	if _, err := f.service.ResolveWithReadings(t.Context(), evidence); err == nil || !strings.Contains(err.Error(), "still require dispatcher reconciliation") {
		t.Fatalf("ResolveWithReadings with an unresolved readable command = %v, want the refusal", err)
	}
	if _, err := f.db.ExecContext(t.Context(), `UPDATE commands SET status = 'succeeded' WHERE command_id = 'cmd-readable'`); err != nil {
		t.Fatal(err)
	}
	if cleared, err := f.service.ResolveWithReadings(t.Context(), evidence); err != nil || !cleared {
		t.Fatalf("ResolveWithReadings with only an unresolved no-readback command = %v, %v; want it cleared", cleared, err)
	}
	f.assertRequired(t, bootA, false)
}

func TestResolveWithReadingsRefusesIncompleteEvidenceAndAStateThatIsNotTheLatest(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.recordState(t, bootA)
	state, _ := f.recordState(t, bootB)
	evidence := f.readingsOf(state, bootB, f.clock.Now())
	evidence.Outputs = nil
	if _, err := f.service.ResolveWithReadings(t.Context(), evidence); err == nil || !strings.Contains(err.Error(), "outputs are required") {
		t.Fatalf("evidence without an output = %v, want the refusal", err)
	}
	evidence = f.readingsOf(map[string]any{"device_id": bootB.DeviceID, "boot_id": bootB.BootID}, bootB, f.clock.Now())
	if _, err := f.service.ResolveWithReadings(t.Context(), evidence); err == nil || !strings.Contains(err.Error(), "latest device state") {
		t.Fatalf("evidence for another state = %v, want the refusal", err)
	}
	other := f.readingsOf(state, bootB, f.clock.Now())
	other.Owner.Instance = "instance-2"
	if _, err := f.service.ResolveWithReadings(t.Context(), other); err == nil || errors.Is(err, domain.ErrNoOpenReconciliation) {
		t.Fatalf("evidence of another owner instance = %v, want the owner refusal", err)
	}
}

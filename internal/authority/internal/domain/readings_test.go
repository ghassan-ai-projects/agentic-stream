package domain

import (
	"strings"
	"testing"
	"time"
)

var openedAt = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func levelReading(change func(*Reading)) Reading {
	reading := Reading{Output: "level_percent", Value: 45, Good: true, BootID: bootOne.BootID, ReceivedAt: openedAt.Add(time.Second)}
	if change != nil {
		change(&reading)
	}
	return reading
}

func levelEvidence(readings ...Reading) ReadingEvidence {
	return ReadingEvidence{Device: bootOne, Owner: ownerA, Source: "plantwire.reading",
		State: map[string]any{"device_id": bootOne.DeviceID, "boot_id": bootOne.BootID}, Outputs: []string{"level_percent"}, Readings: readings}
}

func TestAReadingClearsAReconciliationOnlyWhenItIsGoodLiveOnTheBootAndNewerThanTheOpening(t *testing.T) {
	t.Parallel()
	unknown, reboot := Opening{At: openedAt}, Opening{At: openedAt, Reboot: true}
	cases := []struct {
		name    string
		opening Opening
		reading Reading
		clears  bool
	}{
		{"a good reading after the opening", unknown, levelReading(nil), true},
		{"a reading received before the opening", unknown, levelReading(func(r *Reading) { r.ReceivedAt = openedAt.Add(-time.Second) }), false},
		{"a reading received at the opening", unknown, levelReading(func(r *Reading) { r.ReceivedAt = openedAt }), false},
		{"a reading of a reboot's boot received before the reboot was recorded", reboot, levelReading(func(r *Reading) { r.ReceivedAt = openedAt.Add(-time.Minute) }), true},
		{"a reading that is not good", reboot, levelReading(func(r *Reading) { r.Good = false }), false},
		{"a historical reading", reboot, levelReading(func(r *Reading) { r.Historical = true }), false},
		{"a reading of another boot", reboot, levelReading(func(r *Reading) { r.BootID = "boot-0" }), false},
		{"a reading of another output", reboot, levelReading(func(r *Reading) { r.Output = "state" }), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			request, clears, err := levelEvidence(tc.reading).Resolution(tc.opening)
			if err != nil || clears != tc.clears {
				t.Fatalf("Resolution = %v, %v; want clears %v", clears, err, tc.clears)
			}
			if clears && (request.Outcome != ResolutionSucceeded || request.Device != bootOne) {
				t.Fatalf("request = %+v, want succeeded for %v", request, bootOne)
			}
		})
	}
}

func TestEveryCommandedOutputNeedsItsOwnReading(t *testing.T) {
	t.Parallel()
	evidence := levelEvidence(levelReading(nil))
	evidence.Outputs = []string{"level_percent", "state"}
	if _, clears, err := evidence.Resolution(Opening{At: openedAt}); err != nil || clears {
		t.Fatalf("Resolution without a reading of state = %v, %v; want it not cleared", clears, err)
	}
	evidence.Readings = append(evidence.Readings, levelReading(func(r *Reading) { r.Output, r.Value = "state", "alert" }))
	if _, clears, err := evidence.Resolution(Opening{At: openedAt}); err != nil || !clears {
		t.Fatalf("Resolution with both readings = %v, %v; want it cleared", clears, err)
	}
	if (ReadingEvidence{Device: bootOne, Owner: ownerA, Source: "s", State: evidence.State}).Complete() {
		t.Fatal("evidence without an output is complete; a device with nothing to read back must never clear")
	}
}

func TestReadingEvidenceIsSealedForTheDeviceBootWithTheReadingsItUsed(t *testing.T) {
	t.Parallel()
	request, _, err := levelEvidence(levelReading(nil)).Resolution(Opening{At: openedAt})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseReconciliationEvidence(request.Evidence, bootOne)
	if err != nil {
		t.Fatalf("the sealed evidence does not parse for its boot: %v", err)
	}
	readings := parsed.Feedback["readings"].([]any)
	if parsed.Source != "plantwire.reading" || len(readings) != 1 || readings[0].(map[string]any)["value"] != 45 ||
		parsed.Feedback["opened_at"] != "2026-10-20T12:00:00.000000000Z" {
		t.Fatalf("feedback = %v, want the reading and the opening", parsed.Feedback)
	}
}

func TestAnOpeningKnowsWhetherARebootOpenedIt(t *testing.T) {
	t.Parallel()
	reboot, err := NewOpening(openedAt, []byte(`{"reason":"device_rebooted","previous_boot_id":"boot-0"}`))
	if err != nil || !reboot.Reboot || !reboot.At.Equal(openedAt) {
		t.Fatalf("reboot opening = %+v, %v", reboot, err)
	}
	if unknown, err := NewOpening(openedAt, []byte(`{"reason":"plant wire command outcome is unknown"}`)); err != nil || unknown.Reboot {
		t.Fatalf("unknown-outcome opening = %+v, %v", unknown, err)
	}
	if _, err := NewOpening(openedAt, []byte(`not json`)); err == nil || !strings.Contains(err.Error(), "read the reconciliation opening") {
		t.Fatalf("an unreadable opening = %v, want the refusal", err)
	}
}

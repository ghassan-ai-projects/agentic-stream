package domain

import (
	"testing"
	"time"
)

func TestSafeStopStagesAreTheKnownOnes(t *testing.T) {
	t.Parallel()
	for _, stage := range SafeStopStages {
		if !stage.Valid() {
			t.Errorf("%s is not valid", stage)
		}
	}
	if SafeStopStage("safe_stop_cleared").Valid() {
		t.Error("an unknown stage is valid")
	}
}

func TestASafeStopEventCarriesItsStageAndAlwaysHasDetails(t *testing.T) {
	t.Parallel()
	claim := TargetClaim{Target: "fan-01", Device: bootOne, Owner: ownerA}
	event := SafeStopEvent(claim, SafeStopFailed, nil, testNow)
	if event.Type != "safe_stop_failed" || event.Details == nil || event.Subject != claim {
		t.Fatalf("safe-stop event = %+v, want type safe_stop_failed with non-nil details for %+v", event, claim)
	}
}

func TestASafetyEventIsValidatedAndDefaulted(t *testing.T) {
	t.Parallel()
	complete := map[string]any{"evidence_complete": true, "source": "independent-feedback", "evidence_digest": digestOther}
	tests := []struct {
		name    string
		event   SafetyEvent
		wantErr bool
	}{
		{"zero-tolerance event", SafetyEvent{Type: SafetyUnsafeOutput, Target: "fan-01"}, false},
		{"complete physical evidence", SafetyEvent{Type: SafetyPhysicalTransition, Target: "fan-01", Details: complete}, false},
		{"incomplete physical evidence is allowed", SafetyEvent{Type: SafetyPhysicalTransition, Target: "fan-01", Details: map[string]any{"source": "x"}}, false},
		{"complete claim without digest", SafetyEvent{Type: SafetyPhysicalTransition, Target: "fan-01", Details: map[string]any{"evidence_complete": true, "source": "x"}}, true},
		{"unknown type", SafetyEvent{Type: "unknown", Target: "fan-01"}, true},
		{"missing target", SafetyEvent{Type: SafetyUnsafeOutput}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prepared, err := PrepareSafetyEvent(tt.event, testNow)
			if (err != nil) != tt.wantErr {
				t.Fatalf("PrepareSafetyEvent = %v", err)
			}
			if err == nil && (prepared.Details == nil || !prepared.Occurred.Equal(testNow)) {
				t.Fatalf("defaults not applied: %+v", prepared)
			}
		})
	}
}

func TestASafetyEventKeepsAnExplicitOccurrenceTime(t *testing.T) {
	t.Parallel()
	explicit := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	prepared, err := PrepareSafetyEvent(SafetyEvent{Type: SafetyUnsafeOutput, Target: "fan-01", Occurred: explicit}, testNow)
	if err != nil || !prepared.Occurred.Equal(explicit) {
		t.Fatalf("occurred = %v, %v; want the explicit %v", prepared.Occurred, err, explicit)
	}
}

func TestPhysicalEvidenceIsCompleteOnlyWhenClaimedWithSourceAndDigest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		details map[string]any
		want    bool
	}{
		{"claimed with source and digest", map[string]any{"evidence_complete": true, "source": "s", "evidence_digest": digestOther}, true},
		{"not claimed complete", map[string]any{"source": "x", "evidence_digest": digestOther}, false},
		{"claimed without a digest", map[string]any{"evidence_complete": true, "source": "s"}, false},
		{"claimed without a source", map[string]any{"evidence_complete": true, "evidence_digest": digestOther}, false},
		{"claimed false", map[string]any{"evidence_complete": false, "source": "s", "evidence_digest": digestOther}, false},
	}
	for _, tt := range tests {
		if got := PhysicalEvidenceComplete(tt.details); got != tt.want {
			t.Errorf("%s: PhysicalEvidenceComplete = %t, want %t", tt.name, got, tt.want)
		}
	}
}

func TestSafetyEventsAreTalliedByTypeAndEvidenceCompleteness(t *testing.T) {
	t.Parallel()
	complete := map[string]any{"evidence_complete": true, "source": "s", "evidence_digest": digestOther}
	record := TallySafetyEvents([]SafetyEvent{
		{Type: SafetyUnsafeOutput}, {Type: SafetyUnsafeOutput},
		{Type: SafetyPhysicalTransition, Details: complete},
		{Type: SafetyPhysicalTransition, Details: map[string]any{"evidence_complete": false}},
	})
	if record.EventCounts[SafetyUnsafeOutput] != 2 || record.PhysicalTransitions != 2 || record.CompleteTransitions != 1 {
		t.Fatalf("record = %+v", record)
	}
}

package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewDeviceStateRequiresIdentityAndCanonicalizes(t *testing.T) {
	t.Parallel()
	state, err := NewDeviceState(map[string]any{"boot_id": "boot-1", "device_id": "thermal-01", "safe_state": true})
	if err != nil {
		t.Fatal(err)
	}
	if state.Device != bootOne || string(state.JSON) != `{"boot_id":"boot-1","device_id":"thermal-01","safe_state":true}` || len(state.SHA256) != 32 {
		t.Fatalf("state = %+v", state)
	}
	if _, err := NewDeviceState(map[string]any{"device_id": "thermal-01"}); err == nil {
		t.Fatal("state without boot was accepted")
	}
	if _, err := NewDeviceState(map[string]any{"device_id": "thermal-01", "boot_id": "b", "bad": func() {}}); err == nil {
		t.Fatal("non-JSON state was accepted")
	}
}

func TestObserveState(t *testing.T) {
	t.Parallel()
	clearState := &Reconciliation{Device: bootOne, Status: ReconciliationClear}
	required := &Reconciliation{Device: bootOne, Status: ReconciliationRequired}
	rebooted := DeviceBoot{DeviceID: bootOne.DeviceID, BootID: "boot-2"}
	tests := []struct {
		name     string
		recorded *Reconciliation
		reported DeviceBoot
		want     StateTransition
	}{
		{"first state is clear", nil, bootOne, StateTransition{Change: StateFirstSeen}},
		{"same boot stays clear", clearState, bootOne, StateTransition{Change: StateRefreshed}},
		{"same boot stays required", required, bootOne, StateTransition{Change: StateRefreshed, Required: true}},
		{"reboot requires reconciliation", clearState, rebooted, StateTransition{Change: StateRebooted, Required: true, PreviousBootID: "boot-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ObserveState(tt.recorded, tt.reported); got != tt.want {
				t.Fatalf("ObserveState = %+v, want %+v", got, tt.want)
			}
		})
	}
	event := RebootEvent(StateTransition{Change: StateRebooted, PreviousBootID: "boot-0"}, bootOne, ownerA, testNow)
	if event.Type != EventReconciliationOpened || event.Details["reason"] != "device_rebooted" || event.Subject.Target != bootOne.DeviceID {
		t.Fatalf("reboot event = %+v", event)
	}
}

func TestCheckOpenable(t *testing.T) {
	t.Parallel()
	if _, err := CheckOpenable(nil, bootOne); !errors.Is(err, ErrNoRecordedState) {
		t.Fatalf("no state = %v", err)
	}
	old := DeviceBoot{DeviceID: bootOne.DeviceID, BootID: "boot-0"}
	if _, err := CheckOpenable(&Reconciliation{Device: bootOne}, old); err == nil || !strings.Contains(err.Error(), "does not match current device boot") {
		t.Fatalf("old boot = %v", err)
	}
	already, err := CheckOpenable(&Reconciliation{Device: bootOne, Status: ReconciliationRequired}, bootOne)
	if err != nil || !already {
		t.Fatalf("already open = %v, %v", already, err)
	}
	if event := OpeningEvent(bootOne, ownerA, "unknown receipt", testNow); event.Details["reason"] != "unknown receipt" {
		t.Fatalf("opening event = %+v", event)
	}
}

func TestResolutionOutcomes(t *testing.T) {
	t.Parallel()
	for outcome, status := range map[ResolutionOutcome]ReconciliationStatus{
		ResolutionSucceeded: ReconciliationClear, ResolutionFailed: ReconciliationClear, ResolutionManualReview: ReconciliationRequired,
	} {
		if !outcome.Valid() || outcome.StatusAfter() != status {
			t.Fatalf("%s: valid=%v status=%s", outcome, outcome.Valid(), outcome.StatusAfter())
		}
	}
	if ResolutionOutcome("cleared").Valid() {
		t.Fatal("unknown outcome is valid")
	}
}

func TestCheckResolvable(t *testing.T) {
	t.Parallel()
	evidence := validEvidence(t, bootOne, "fan-01")
	stateSHA, err := canonicalSHA(evidence["state"])
	if err != nil {
		t.Fatal(err)
	}
	open := &Reconciliation{Device: bootOne, Status: ReconciliationRequired, StateSHA256: stateSHA}
	tests := []struct {
		name     string
		recorded *Reconciliation
		evidence map[string]any
		want     error
		wantText string
	}{
		{"open reconciliation with latest state", open, evidence, nil, ""},
		{"no recorded state", nil, evidence, ErrNoRecordedState, ""},
		{"clear device", &Reconciliation{Device: bootOne, Status: ReconciliationClear}, evidence, ErrNoOpenReconciliation, ""},
		{"other boot", &Reconciliation{Device: DeviceBoot{DeviceID: "thermal-01", BootID: "boot-9"}, Status: ReconciliationRequired}, evidence, ErrNoOpenReconciliation, ""},
		{"stale state", &Reconciliation{Device: bootOne, Status: ReconciliationRequired, StateSHA256: make([]byte, 32)}, evidence, nil, "does not bind the latest device state"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := CheckResolvable(tt.recorded, bootOne, tt.evidence)
			if tt.wantText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantText) {
					t.Fatalf("CheckResolvable = %v, want %q", err, tt.wantText)
				}
				return
			}
			if !errors.Is(err, tt.want) || (tt.want == nil && err != nil) {
				t.Fatalf("CheckResolvable = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestStateEvidenceMustMatchItsOwnDigest(t *testing.T) {
	t.Parallel()
	evidence := validEvidence(t, bootOne, "fan-01")
	stateSHA, err := canonicalSHA(evidence["state"])
	if err != nil {
		t.Fatal(err)
	}
	evidence["state"] = map[string]any{"device_id": "thermal-01", "boot_id": "boot-1", "safe_state": false}
	err = CheckResolvable(&Reconciliation{Device: bootOne, Status: ReconciliationRequired, StateSHA256: stateSHA}, bootOne, evidence)
	if err == nil || !strings.Contains(err.Error(), "does not match typed state evidence") {
		t.Fatalf("tampered state = %v", err)
	}
}

func TestResolutionRecordsCanonicalEvidence(t *testing.T) {
	t.Parallel()
	evidence := validEvidence(t, bootOne, "fan-01")
	resolution, err := NewResolution(bootOne, ownerA, ResolutionManualReview, evidence)
	if err != nil {
		t.Fatal(err)
	}
	event := ResolutionEvent(resolution, testNow)
	if event.Details["barrier_cleared"] != false || event.Details["final_status"] != "manual_review" || len(resolution.EvidenceSHA256) != 32 {
		t.Fatalf("resolution event = %+v", event)
	}
	if _, err := NewResolution(bootOne, ownerA, ResolutionSucceeded, map[string]any{"source": "x"}); err == nil {
		t.Fatal("invalid evidence produced a resolution")
	}
	if err := CheckCommandsReconciled(2); err == nil || CheckCommandsReconciled(0) != nil {
		t.Fatal("unresolved command check")
	}
}

func canonicalSHA(value any) ([]byte, error) {
	state, err := NewDeviceState(value.(map[string]any))
	return state.SHA256, err
}

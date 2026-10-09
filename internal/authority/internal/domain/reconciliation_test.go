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

func TestObservingADeviceStateOpensReconciliationOnlyForANewBoot(t *testing.T) {
	t.Parallel()
	clearState := &Reconciliation{Device: bootOne, Status: ReconciliationClear}
	required := &Reconciliation{Device: bootOne, Status: ReconciliationRequired}
	current := mustState(t, bootOne)
	rebooted := mustState(t, DeviceBoot{DeviceID: bootOne.DeviceID, BootID: "boot-2"})
	tests := []struct {
		name           string
		recorded       *Reconciliation
		reported       DeviceState
		change         StateChange
		required       bool
		previousBootID string
	}{
		{"first state is clear", nil, current, StateFirstSeen, false, ""},
		{"same boot stays clear", clearState, current, StateRefreshed, false, ""},
		{"same boot stays required", required, current, StateRefreshed, true, ""},
		{"reboot requires reconciliation", clearState, rebooted, StateRebooted, true, "boot-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ObserveState(tt.recorded, tt.reported, ownerA)
			if got.Change != tt.change || got.Required != tt.required || got.PreviousBootID != tt.previousBootID || got.Owner != ownerA {
				t.Fatalf("ObserveState = %+v", got)
			}
		})
	}
}

func TestARebootIsAuditedAsAnOpenedReconciliationWithItsReason(t *testing.T) {
	t.Parallel()
	event := RebootEvent(StateObservation{State: mustState(t, bootOne), Owner: ownerA, Change: StateRebooted, PreviousBootID: "boot-0"}, testNow)
	if event.Type != EventReconciliationOpened || event.Details["reason"] != "device_rebooted" || event.Subject.Target != bootOne.DeviceID {
		t.Fatalf("reboot event = %+v, want %s with reason device_rebooted for %s", event, EventReconciliationOpened, bootOne.DeviceID)
	}
}

func TestOnlyTheCurrentBootOfAKnownDeviceCanHaveAReconciliationOpened(t *testing.T) {
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
}

func TestAnOpeningIsAuditedWithTheReasonGiven(t *testing.T) {
	t.Parallel()
	event := OpeningEvent(ReconciliationOpening{Device: bootOne, Owner: ownerA, Reason: "unknown receipt"}, testNow)
	if event.Type != EventReconciliationOpened || event.Details["reason"] != "unknown receipt" {
		t.Fatalf("opening event = %+v, want %s with the reason given", event, EventReconciliationOpened)
	}
}

func TestResolutionOutcomesClearOrKeepTheBarrier(t *testing.T) {
	t.Parallel()
	for outcome, status := range map[ResolutionOutcome]ReconciliationStatus{
		ResolutionSucceeded: ReconciliationClear, ResolutionFailed: ReconciliationClear, ResolutionManualReview: ReconciliationRequired,
	} {
		if !outcome.Valid() || outcome.StatusAfter() != status {
			t.Errorf("%s: valid=%t status after=%s, want valid and %s", outcome, outcome.Valid(), outcome.StatusAfter(), status)
		}
	}
	if ResolutionOutcome("cleared").Valid() {
		t.Error("an unknown outcome is valid")
	}
}

func TestAReconciliationIsResolvableOnlyWhileOpenAndWithEvidenceOfTheLatestState(t *testing.T) {
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
			err := CheckResolvable(tt.recorded, Resolution{Device: bootOne, Evidence: mustParse(t, tt.evidence)})
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
	tampered := mustParse(t, evidence)
	tampered.State = map[string]any{"device_id": "thermal-01", "boot_id": "boot-1", "safe_state": false}
	err = CheckResolvable(&Reconciliation{Device: bootOne, Status: ReconciliationRequired, StateSHA256: stateSHA}, Resolution{Device: bootOne, Evidence: tampered})
	if err == nil || !strings.Contains(err.Error(), "does not match typed state evidence") {
		t.Fatalf("tampered state = %v", err)
	}
}

func TestResolutionRecordsCanonicalEvidence(t *testing.T) {
	t.Parallel()
	evidence := validEvidence(t, bootOne, "fan-01")
	resolution, err := NewResolution(ResolutionRequest{Device: bootOne, Owner: ownerA, Outcome: ResolutionManualReview, Evidence: evidence})
	if err != nil {
		t.Fatal(err)
	}
	event := ResolutionEvent(resolution, testNow)
	if event.Details["barrier_cleared"] != false || event.Details["final_status"] != "manual_review" || len(resolution.EvidenceSHA256) != 32 {
		t.Fatalf("resolution event = %+v", event)
	}
	if _, err := NewResolution(ResolutionRequest{Device: bootOne, Owner: ownerA, Outcome: ResolutionSucceeded, Evidence: map[string]any{"source": "x"}}); err == nil {
		t.Fatal("invalid evidence produced a resolution")
	}
}

func TestResolutionWaitsForEveryBoundCommandToBeReconciled(t *testing.T) {
	t.Parallel()
	if err := CheckCommandsReconciled(0); err != nil {
		t.Errorf("no unresolved commands: %v", err)
	}
	if err := CheckCommandsReconciled(2); err == nil || !strings.Contains(err.Error(), "still require dispatcher reconciliation") {
		t.Errorf("two unresolved commands: %v, want a refusal that names the dispatcher", err)
	}
}

func canonicalSHA(value any) ([]byte, error) {
	state, err := NewDeviceState(value.(map[string]any))
	return state.SHA256, err
}

func mustParse(t *testing.T, document map[string]any) ReconciliationEvidence {
	t.Helper()
	evidence, err := ParseReconciliationEvidence(document, bootOne)
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}

func mustState(t *testing.T, device DeviceBoot) DeviceState {
	t.Helper()
	state, err := NewDeviceState(map[string]any{"device_id": device.DeviceID, "boot_id": device.BootID})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestAResolutionRequestNeedsItsOutcomeEvidenceAndDevice(t *testing.T) {
	t.Parallel()
	evidence := map[string]any{"source": "s"}
	for _, tt := range []struct {
		name    string
		request ResolutionRequest
		wantErr bool
	}{
		{"complete", ResolutionRequest{Device: bootOne, Outcome: ResolutionSucceeded, Evidence: evidence}, false},
		{"unknown outcome", ResolutionRequest{Device: bootOne, Outcome: "cleared", Evidence: evidence}, true},
		{"no evidence", ResolutionRequest{Device: bootOne, Outcome: ResolutionFailed}, true},
		{"partial device", ResolutionRequest{Device: DeviceBoot{DeviceID: "d"}, Outcome: ResolutionFailed, Evidence: evidence}, true},
	} {
		if err := tt.request.Check(); (err != nil) != tt.wantErr {
			t.Errorf("%s: Check = %v", tt.name, err)
		}
	}
}

func TestAnOpeningNeedsAReason(t *testing.T) {
	t.Parallel()
	if (ReconciliationOpening{Device: bootOne, Owner: ownerA}).Complete() {
		t.Error("an opening without a reason is complete")
	}
	if !(ReconciliationOpening{Device: bootOne, Owner: ownerA, Reason: "r"}).Complete() {
		t.Error("an opening with a device, owner and reason is incomplete")
	}
}

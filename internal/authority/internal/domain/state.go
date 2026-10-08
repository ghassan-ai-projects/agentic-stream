package domain

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// DeviceState is the typed state document a device reports on handshake or
// query, in canonical JSON with its raw SHA-256.
type DeviceState struct {
	Device DeviceBoot
	JSON   []byte
	SHA256 []byte
}

// NewDeviceState canonicalizes a reported state document. It must name its
// device and boot.
func NewDeviceState(document map[string]any) (DeviceState, error) {
	deviceID, _ := document["device_id"].(string)
	bootID, _ := document["boot_id"].(string)
	device := DeviceBoot{DeviceID: deviceID, BootID: bootID}
	if !device.Complete() {
		return DeviceState{}, fmt.Errorf("device state must name its device_id and boot_id")
	}
	stateJSON, err := canonicaljson.Marshal(document)
	if err != nil {
		return DeviceState{}, fmt.Errorf("canonicalize device state: %w", err)
	}
	return DeviceState{Device: device, JSON: stateJSON, SHA256: canonicaljson.Sum(stateJSON)}, nil
}

// StateChange classifies a reported device state against what is recorded.
type StateChange string

// State changes.
const (
	StateFirstSeen StateChange = "first_seen"
	StateRefreshed StateChange = "refreshed"
	StateRebooted  StateChange = "rebooted"
)

// StateObservation is one reported device state and what it does to the
// device's reconciliation.
type StateObservation struct {
	State          DeviceState
	Owner          Owner
	Change         StateChange
	Required       bool
	PreviousBootID string
}

// ObserveState decides what a reported state does to the device's
// reconciliation: the first state is clear, the same boot keeps its status,
// and a reboot requires reconciliation.
func ObserveState(recorded *Reconciliation, state DeviceState, owner Owner) StateObservation {
	observation := StateObservation{State: state, Owner: owner}
	switch {
	case recorded == nil:
		observation.Change = StateFirstSeen
	case recorded.Device.BootID != state.Device.BootID:
		observation.Change, observation.Required, observation.PreviousBootID = StateRebooted, true, recorded.Device.BootID
	default:
		observation.Change, observation.Required = StateRefreshed, recorded.Required()
	}
	return observation
}

// RebootEvent is the audit record of the reconciliation a reboot opens.
func RebootEvent(observation StateObservation, now time.Time) AuthorityEvent {
	return newEvent(EventReconciliationOpened, DeviceSubject(observation.State.Device, observation.Owner), map[string]any{
		"reason": "device_rebooted", "previous_boot_id": observation.PreviousBootID,
	}, now)
}

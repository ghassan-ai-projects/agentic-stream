package domain

import (
	"crypto/sha256"
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
	sum := sha256.Sum256(stateJSON)
	return DeviceState{Device: device, JSON: stateJSON, SHA256: sum[:]}, nil
}

// StateChange classifies a reported device state against what is recorded.
type StateChange string

// State changes.
const (
	StateFirstSeen StateChange = "first_seen"
	StateRefreshed StateChange = "refreshed"
	StateRebooted  StateChange = "rebooted"
)

// StateTransition is the effect of one reported device state.
type StateTransition struct {
	Change         StateChange
	Required       bool
	PreviousBootID string
}

// ObserveState decides what a reported state does to the device's
// reconciliation: the first state is clear, the same boot keeps its status,
// and a reboot requires reconciliation.
func ObserveState(recorded *Reconciliation, reported DeviceBoot) StateTransition {
	switch {
	case recorded == nil:
		return StateTransition{Change: StateFirstSeen}
	case recorded.Device.BootID != reported.BootID:
		return StateTransition{Change: StateRebooted, Required: true, PreviousBootID: recorded.Device.BootID}
	default:
		return StateTransition{Change: StateRefreshed, Required: recorded.Required()}
	}
}

// RebootEvent is the audit record of the reconciliation a reboot opens.
func RebootEvent(transition StateTransition, device DeviceBoot, owner Owner, now time.Time) AuthorityEvent {
	return newEvent(EventReconciliationOpened, DeviceSubject(device, owner), map[string]any{
		"reason": "device_rebooted", "previous_boot_id": transition.PreviousBootID,
	}, now)
}

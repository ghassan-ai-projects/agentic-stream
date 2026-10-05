package device

import (
	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
)

func validateDeviceState(frame []byte, catalog *CapabilityCatalog, catalogDigest string, allowedCapabilityDigests, allowedFirmwareDigests []string) (map[string]any, error) {
	state, err := DecodeDeviceRecord(frame)
	if err != nil {
		return nil, err
	}
	if err := domain.CheckState(state, domain.StateAllowlist{
		ProtocolVersion: catalog.ProtocolVersion, CatalogDigest: catalogDigest,
		CapabilityDigest: allowedCapabilityDigests, FirmwareDigests: allowedFirmwareDigests,
	}); err != nil {
		return nil, err //nolint:wrapcheck // the caller names the handshake or refresh.
	}
	return state, nil
}

func stateString(state map[string]any, key string) string {
	value, _ := state[key].(string)
	return value
}

type cachedReceipt struct {
	commandDigest string
	receipt       map[string]any
	result        map[string]any
}

type deviceExchangeError struct{ err error }

func (e *deviceExchangeError) Error() string { return e.err.Error() }
func (e *deviceExchangeError) Unwrap() error { return e.err }

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func cloneDocument(document map[string]any) map[string]any {
	clone := make(map[string]any, len(document))
	for key, value := range document {
		clone[key] = value
	}
	return clone
}

// owner is the runtime owner the session commands as.
func (s *DeviceSession) owner() deviceauthority.Owner {
	return deviceauthority.Owner{Epoch: s.ownerEpoch, Instance: s.ownerInstance}
}

// deviceBoot is the device boot the session is currently bound to.
func (s *DeviceSession) deviceBoot() deviceauthority.DeviceBoot {
	return deviceauthority.DeviceBoot{DeviceID: s.deviceID, BootID: s.bootID}
}

// targetClaim is the session's claim on one target of the current boot.
func (s *DeviceSession) targetClaim(target string) deviceauthority.TargetClaim {
	return deviceauthority.TargetClaim{Target: target, Device: s.deviceBoot(), Owner: s.owner()}
}

// reconciliationOpening asks to open a reconciliation for the current boot.
func (s *DeviceSession) reconciliationOpening(reason string) deviceauthority.ReconciliationOpening {
	return deviceauthority.ReconciliationOpening{Device: s.deviceBoot(), Owner: s.owner(), Reason: reason}
}

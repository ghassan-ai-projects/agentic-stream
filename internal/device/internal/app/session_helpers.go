package app

import (
	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/wire"
)

// decodeState decodes a state record and requires it to match the catalog
// and the firmware allow-list.
func decodeState(frame []byte, catalog *domain.CapabilityCatalog, catalogDigest string, allowedFirmwareDigests []string) (domain.State, error) {
	state, err := wire.DecodeState(frame)
	if err != nil {
		return domain.State{}, err //nolint:wrapcheck // the caller names the handshake or refresh.
	}
	if err := domain.CheckState(state, domain.StateAllowlist{
		ProtocolVersion: catalog.ProtocolVersion, CatalogDigest: catalogDigest,
		CapabilityDigest: []string{catalogDigest}, FirmwareDigests: allowedFirmwareDigests,
	}); err != nil {
		return domain.State{}, err //nolint:wrapcheck // the caller names the handshake or refresh.
	}
	return state, nil
}

// cachedExchange is the first answer to an idempotency key on this boot.
type cachedExchange struct {
	commandIdentity string
	exchange        Exchange
}

type deviceExchangeError struct{ err error }

func (e *deviceExchangeError) Error() string { return e.err.Error() }
func (e *deviceExchangeError) Unwrap() error { return e.err }

// owner is the runtime owner the session commands as.
func (s *Session) owner() deviceauthority.Owner {
	return deviceauthority.Owner{Epoch: s.ownerEpoch, Instance: s.ownerInstance}
}

// deviceBoot is the device boot the session is currently bound to.
func (s *Session) deviceBoot() deviceauthority.DeviceBoot {
	return deviceauthority.DeviceBoot{DeviceID: s.deviceID, BootID: s.bootID}
}

// targetClaim is the session's claim on one target of the current boot.
func (s *Session) targetClaim(target string) deviceauthority.TargetClaim {
	return deviceauthority.TargetClaim{Target: target, Device: s.deviceBoot(), Owner: s.owner()}
}

// reconciliationOpening asks to open a reconciliation for the current boot.
func (s *Session) reconciliationOpening(reason string) deviceauthority.ReconciliationOpening {
	return deviceauthority.ReconciliationOpening{Device: s.deviceBoot(), Owner: s.owner(), Reason: reason}
}

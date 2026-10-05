package device

import (
	"context"
	"fmt"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// GatewayEffectorConfig configures a serial effector around an already-open
// typed gateway link. The caller owns the transport before this function is
// called; the returned close function owns it after a successful handshake.
type GatewayEffectorConfig struct {
	Transport              DeviceTransport
	Catalog                *CapabilityCatalog
	AllowedFirmwareDigests []string
	OwnerEpoch             string
	OwnerInstance          string
	Authority              *deviceauthority.Service
	Telemetry              *telemetry.Runtime
}

// NewGatewayEffector opens a governed serial effector over a typed gateway
// link. It performs the capability and device-state handshake before exposing
// the effector to the action plane.
func NewGatewayEffector(ctx context.Context, config GatewayEffectorConfig) (*SerialEffector, func() error, error) {
	if config.Transport == nil {
		return nil, nil, fmt.Errorf("gateway effector requires a device transport")
	}
	if config.Catalog == nil {
		return nil, nil, fmt.Errorf("gateway effector requires a capability catalog")
	}
	catalogDigest, err := config.Catalog.Digest()
	if err != nil {
		return nil, nil, fmt.Errorf("digest device capability catalog: %w", err)
	}
	return openGatewayEffector(ctx, config, catalogDigest)
}

func openGatewayEffector(ctx context.Context, config GatewayEffectorConfig, catalogDigest string) (*SerialEffector, func() error, error) {
	session, err := OpenDeviceSession(ctx, gatewaySessionConfig(config, catalogDigest))
	if err != nil {
		return nil, nil, fmt.Errorf("open device session: %w", err)
	}
	return exposeSessionEffector(session, config)
}

func gatewaySessionConfig(config GatewayEffectorConfig, catalogDigest string) DeviceSessionConfig {
	return DeviceSessionConfig{
		Transport:                config.Transport,
		Catalog:                  config.Catalog,
		AllowedCapabilityDigests: []string{catalogDigest},
		AllowedFirmwareDigests:   config.AllowedFirmwareDigests,
		OwnerEpoch:               config.OwnerEpoch,
		OwnerInstance:            config.OwnerInstance,
		Authority:                config.Authority,
		Telemetry:                config.Telemetry,
	}
}

func exposeSessionEffector(session *DeviceSession, config GatewayEffectorConfig) (*SerialEffector, func() error, error) {
	effector := NewSerialEffector(session, config.Catalog)
	if config.Telemetry != nil {
		effector = effector.WithTelemetry(config.Telemetry)
	}
	return effector, session.Close, nil
}

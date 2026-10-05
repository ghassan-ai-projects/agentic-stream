package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// GatewayConfig configures a serial effector around an already-open
// typed gateway link. The caller owns the transport before this function is
// called; the returned close function owns it after a successful handshake.
type GatewayConfig struct {
	Transport              Transport
	Catalog                *domain.CapabilityCatalog
	AllowedFirmwareDigests []string
	OwnerEpoch             string
	OwnerInstance          string
	Authority              *deviceauthority.Service
	Telemetry              *telemetry.Runtime
}

// OpenGatewayEffector opens a governed serial effector over a typed gateway
// link. It performs the capability and device-state handshake before exposing
// the effector to the action plane.
func OpenGatewayEffector(ctx context.Context, config GatewayConfig) (*GatewayEffector, func() error, error) {
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
	return openGatewaySession(ctx, config, catalogDigest)
}

func openGatewaySession(ctx context.Context, config GatewayConfig, catalogDigest string) (*GatewayEffector, func() error, error) {
	session, err := OpenSession(ctx, gatewaySessionConfig(config, catalogDigest))
	if err != nil {
		return nil, nil, fmt.Errorf("open device session: %w", err)
	}
	return exposeSessionEffector(session, config)
}

func gatewaySessionConfig(config GatewayConfig, catalogDigest string) SessionConfig {
	return SessionConfig{
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

func exposeSessionEffector(session *Session, config GatewayConfig) (*GatewayEffector, func() error, error) {
	effector := NewGatewayEffector(session, config.Catalog)
	effector = effector.WithTelemetry(config.Telemetry)
	return effector, session.Close, nil
}

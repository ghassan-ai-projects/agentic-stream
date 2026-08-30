package actions

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// EmulatorEffectorConfig configures a serial effector wired to a device gateway
// over a Unix domain socket — the emulator effect profile. It carries a typed
// capability catalog and durable authority, never a raw serial path or a model
// credential.
type EmulatorEffectorConfig struct {
	// SocketPath is the device gateway UDS (e.g. the Streams Simulator
	// `streamsim device serve --socket` link).
	SocketPath string
	Catalog    *CapabilityCatalog
	// AllowedFirmwareDigests is the firmware allow-list checked at handshake.
	AllowedFirmwareDigests []string
	AuthorityEpoch         string
	OwnerInstance          string
	Authority              *storage.TargetAuthority
	Reconciliation         *storage.ReconciliationStore
	Telemetry              *telemetry.Runtime
}

// GatewayEffectorConfig configures a serial effector around an already-open
// typed gateway link. The caller owns the transport before this function is
// called; the returned close function owns it after a successful handshake.
type GatewayEffectorConfig struct {
	Transport              DeviceTransport
	Catalog                *CapabilityCatalog
	AllowedFirmwareDigests []string
	AuthorityEpoch         string
	OwnerInstance          string
	Authority              *storage.TargetAuthority
	Reconciliation         *storage.ReconciliationStore
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
	session, err := OpenDeviceSession(ctx, DeviceSessionConfig{
		Transport:                config.Transport,
		Catalog:                  config.Catalog,
		AllowedCapabilityDigests: []string{catalogDigest},
		AllowedFirmwareDigests:   config.AllowedFirmwareDigests,
		AuthorityEpoch:           config.AuthorityEpoch,
		OwnerInstance:            config.OwnerInstance,
		Authority:                config.Authority,
		Reconciliation:           config.Reconciliation,
		Telemetry:                config.Telemetry,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open device session: %w", err)
	}
	effector := NewSerialEffector(session, config.Catalog)
	if config.Telemetry != nil {
		effector = effector.WithTelemetry(config.Telemetry)
	}
	return effector, session.Close, nil
}

// NewEmulatorEffector dials the device gateway, opens a validated device session
// (reading and checking the opening state handshake), and returns a serial
// effector plus a close function that tears the session and transport down. It
// is the single wiring point the CLI and the HIL-0 harness use to drive the
// emulator; the device must already be listening on SocketPath.
func NewEmulatorEffector(ctx context.Context, config EmulatorEffectorConfig) (*SerialEffector, func() error, error) {
	if config.Catalog == nil {
		return nil, nil, fmt.Errorf("emulator effector requires a capability catalog")
	}
	if config.SocketPath == "" {
		return nil, nil, fmt.Errorf("emulator effector requires a device socket path")
	}
	transport, err := DialUDSTransport(ctx, config.SocketPath)
	if err != nil {
		return nil, nil, err
	}
	effector, closeFn, err := NewGatewayEffector(ctx, GatewayEffectorConfig{
		Transport:              transport,
		Catalog:                config.Catalog,
		AllowedFirmwareDigests: config.AllowedFirmwareDigests,
		AuthorityEpoch:         config.AuthorityEpoch,
		OwnerInstance:          config.OwnerInstance,
		Authority:              config.Authority,
		Reconciliation:         config.Reconciliation,
		Telemetry:              config.Telemetry,
	})
	if err != nil {
		_ = transport.Close()
		return nil, nil, err
	}
	return effector, closeFn, nil
}

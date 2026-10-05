package device

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// Transport is the gateway link a gateway effector sends device records over.
type Transport = app.Transport

// GatewayEffectorConfig configures a gateway effector around an already-open
// gateway link. The caller owns the link until NewGatewayEffector succeeds;
// the returned close function owns it afterwards.
type GatewayEffectorConfig struct {
	Transport              Transport
	Catalog                *CapabilityCatalog
	AllowedFirmwareDigests []string
	OwnerEpoch             string
	OwnerInstance          string
	Authority              *deviceauthority.Service
	Telemetry              *telemetry.Runtime
}

// GatewayEffector delivers approved commands to one device through its
// gateway, after the capability and device-state handshake.
type GatewayEffector struct {
	effector *app.GatewayEffector
}

// NewGatewayEffector performs the device handshake over the gateway link and
// returns the effector with a close function that releases claims and the
// link.
func NewGatewayEffector(ctx context.Context, config GatewayEffectorConfig) (*GatewayEffector, func() error, error) {
	effector, closeFn, err := app.OpenGatewayEffector(ctx, app.GatewayConfig(config))
	if err != nil {
		return nil, nil, err
	}
	return &GatewayEffector{effector: effector}, closeFn, nil
}

// WithTelemetry connects action-boundary counters to runtime telemetry.
func (e *GatewayEffector) WithTelemetry(runtimeTelemetry *telemetry.Runtime) *GatewayEffector {
	e.effector.WithTelemetry(runtimeTelemetry)
	return e
}

// Dispatch materializes and delivers one approved command.
func (e *GatewayEffector) Dispatch(ctx context.Context, command actionport.Command) (actionport.Effect, error) {
	return e.effector.Dispatch(ctx, command)
}

// DispatchAuthorized checks the final authorization immediately before
// materialization and delivery.
func (e *GatewayEffector) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	return e.effector.DispatchAuthorized(ctx, command, authorization)
}

// SafeStop sends the catalog-owned safe-state command for target through the
// priority lane.
func (e *GatewayEffector) SafeStop(ctx context.Context, target string) (actionport.Effect, error) {
	return e.effector.SafeStop(ctx, target)
}

// VerifyDeviceCommand reads a fresh device state and reports whether the
// observed output matches the command, with sealed reconciliation evidence.
func (e *GatewayEffector) VerifyDeviceCommand(ctx context.Context, command actionport.Command) (string, map[string]any, error) {
	return e.effector.VerifyDeviceCommand(ctx, command)
}

// NewSimulatedEffector returns the deterministic effector that accepts each
// idempotency key once and touches no device.
func NewSimulatedEffector() actionport.AuthorizedEffector {
	return app.NewSimulatedEffector()
}

// NewFailClosedEffector returns the fallback that refuses every route no live
// effector owns, so a live profile never turns an unmapped route into a
// simulated success.
func NewFailClosedEffector(profile EffectProfile) actionport.AuthorizedEffector {
	return app.NewFailClosedEffector(profile)
}

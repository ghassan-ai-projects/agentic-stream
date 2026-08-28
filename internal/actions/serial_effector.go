package actions

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// SerialEffector is the governed action-plane adapter for a typed gateway
// link. It never opens a raw serial port; DeviceSession owns that boundary.
type SerialEffector struct {
	session   *DeviceSession
	catalog   *CapabilityCatalog
	telemetry *telemetry.Runtime
}

// NewSerialEffector creates an effector for an already-open device session.
// The catalog must be the same closed catalog whose digest was bound during
// the session handshake.
func NewSerialEffector(session *DeviceSession, catalog *CapabilityCatalog) *SerialEffector {
	return &SerialEffector{session: session, catalog: catalog}
}

// WithTelemetry connects action-boundary counters to the runtime telemetry
// surface. It is optional for embedders and tests.
func (e *SerialEffector) WithTelemetry(runtimeTelemetry *telemetry.Runtime) *SerialEffector {
	if e != nil {
		e.telemetry = runtimeTelemetry
		e.session.WithTelemetry(runtimeTelemetry)
	}
	return e
}

// Dispatch sends one materialized command. Direct callers should prefer
// DispatchAuthorized when an interlock is available; the dispatcher uses the
// authorized method whenever it has a live interlock.
func (e *SerialEffector) Dispatch(ctx context.Context, command Command) (Effect, error) {
	return e.dispatch(ctx, command)
}

// DispatchAuthorized performs the final authorization check immediately
// before materialization and transport delivery.
func (e *SerialEffector) DispatchAuthorized(ctx context.Context, command Command, authorization Authorization) (Effect, error) {
	if authorization.Check == nil {
		return Effect{}, fmt.Errorf("dispatch authorization is required")
	}
	if err := authorization.Check(ctx); err != nil {
		return Effect{}, err
	}
	return e.dispatch(ctx, command)
}

func (e *SerialEffector) dispatch(ctx context.Context, command Command) (Effect, error) {
	if e == nil || e.session == nil || e.catalog == nil {
		return Effect{}, fmt.Errorf("serial effector session and catalog are required")
	}
	catalogDigest, err := e.catalog.Digest()
	if err != nil {
		return Effect{}, fmt.Errorf("validate serial capability catalog: %w", err)
	}
	if catalogDigest != e.session.CapabilityDigest() {
		return Effect{}, fmt.Errorf("serial capability catalog does not match the device session")
	}
	bootID := e.session.BootID()
	wireCommand, err := e.catalog.Materialize(command, bootID)
	if err != nil {
		return Effect{}, fmt.Errorf("materialize serial command: %w", err)
	}
	receipt, sent, err := e.session.Exchange(ctx, wireCommand)
	if err != nil {
		if sent {
			if e.telemetry != nil {
				e.telemetry.ObserveActionUnknownOutcome()
			}
			return Effect{}, &UnknownOutcomeError{Err: err}
		}
		return Effect{}, fmt.Errorf("exchange serial command: %w", err)
	}

	providerResult := map[string]any{"receipt": receipt}
	accepted, _ := receipt["accepted"].(bool)
	effect := Effect{ProviderResult: providerResult, VerificationPending: accepted}
	if accepted && e.telemetry != nil {
		e.telemetry.ObserveVerificationPending()
	}
	if !accepted {
		rejectCode, _ := receipt["reject_code"].(string)
		return effect, fmt.Errorf("device rejected serial command: %s", rejectCode)
	}
	return effect, nil
}

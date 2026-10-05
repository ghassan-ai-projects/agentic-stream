package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// GatewayEffector is the governed action-plane adapter for a typed gateway
// link. It never opens a raw serial port; Session owns that boundary.
type GatewayEffector struct {
	session   *Session
	catalog   *domain.CapabilityCatalog
	telemetry *telemetry.Runtime
}

// NewGatewayEffector creates an effector for an already-open device session.
// The catalog must be the same closed catalog whose digest was bound during
// the session handshake.
func NewGatewayEffector(session *Session, catalog *domain.CapabilityCatalog) *GatewayEffector {
	return &GatewayEffector{session: session, catalog: catalog}
}

// WithTelemetry connects action-boundary counters to the runtime telemetry
// surface. It is optional for embedders and tests.
func (e *GatewayEffector) WithTelemetry(runtimeTelemetry *telemetry.Runtime) *GatewayEffector {
	if e != nil {
		e.telemetry = runtimeTelemetry
		if e.session != nil {
			e.session.WithTelemetry(runtimeTelemetry)
		}
	}
	return e
}

// Dispatch sends one materialized command. Direct callers should prefer
// DispatchAuthorized when an interlock is available; the dispatcher uses the
// authorized method whenever it has a live interlock.
func (e *GatewayEffector) Dispatch(ctx context.Context, command actionport.Command) (actionport.Effect, error) {
	return e.dispatch(ctx, command)
}

// DispatchAuthorized performs the final authorization check immediately
// before materialization and transport delivery.
func (e *GatewayEffector) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	if authorization.Check == nil {
		return actionport.Effect{}, fmt.Errorf("dispatch authorization is required")
	}
	if err := authorization.Check(ctx); err != nil {
		return actionport.Effect{}, fmt.Errorf("%w", err)
	}
	return e.dispatch(ctx, command)
}

func (e *GatewayEffector) dispatch(ctx context.Context, command actionport.Command) (actionport.Effect, error) {
	if e == nil || e.session == nil || e.catalog == nil {
		return actionport.Effect{}, fmt.Errorf("serial effector session and catalog are required")
	}
	catalogDigest, err := e.catalog.Digest()
	if err != nil {
		return actionport.Effect{}, fmt.Errorf("validate serial capability catalog: %w", err)
	}
	if catalogDigest != e.session.CapabilityDigest() {
		return actionport.Effect{}, fmt.Errorf("serial capability catalog does not match the device session")
	}
	return e.dispatchMaterialized(ctx, command)
}

func (e *GatewayEffector) dispatchMaterialized(ctx context.Context, command actionport.Command) (actionport.Effect, error) {
	bootID := e.session.BootID()
	wireCommand, err := e.catalog.Materialize(command, bootID)
	if err != nil {
		return actionport.Effect{}, fmt.Errorf("materialize serial command: %w", err)
	}
	exchange, sent, err := e.session.Exchange(ctx, wireCommand)
	if err != nil {
		return e.failedDeviceEffect(exchange, sent, err)
	}
	return e.acceptedDeviceEffect(exchange)
}

func (e *GatewayEffector) failedDeviceEffect(exchange Exchange, sent bool, err error) (actionport.Effect, error) {
	providerResult := exchange.document()
	if sent {
		e.telemetry.ObserveActionUnknownOutcome()
		return actionport.Effect{ProviderResult: providerResult}, &actionport.UnknownOutcomeError{Err: err}
	}
	return actionport.Effect{}, fmt.Errorf("exchange serial command: %w", err)
}

func (e *GatewayEffector) acceptedDeviceEffect(exchange Exchange) (actionport.Effect, error) {
	providerResult := exchange.document()
	accepted := exchange.Receipt.Accepted
	effect := actionport.Effect{ProviderResult: providerResult, VerificationPending: accepted}
	if accepted {
		e.telemetry.ObserveVerificationPending()
	}
	if !accepted {
		rejectCode := ""
		if exchange.Receipt.RejectCode != nil {
			rejectCode = *exchange.Receipt.RejectCode
		}
		return effect, fmt.Errorf("device rejected serial command: %s", rejectCode)
	}
	return effect, nil
}

// SafeStop requests the catalog-owned safe state through the session priority
// lane. It is intentionally separate from policy-approved ordinary dispatch.
func (e *GatewayEffector) SafeStop(ctx context.Context, target string) (actionport.Effect, error) {
	if e == nil || e.session == nil {
		return actionport.Effect{}, fmt.Errorf("serial effector session is required")
	}
	exchange, sent, err := e.session.SafeStop(ctx, target)
	return classifySafeStop(exchange, sent, err)
}

func classifySafeStop(exchange Exchange, sent bool, err error) (actionport.Effect, error) {
	providerResult := exchange.document()
	switch {
	case err == nil:
		return actionport.Effect{ProviderResult: providerResult, VerificationPending: true}, nil
	case !sent:
		// Nothing reached the device, so the failure is known.
		return actionport.Effect{}, err
	case exchange.Receipt.Document == nil && exchange.Result.Document == nil:
		return actionport.Effect{}, &actionport.UnknownOutcomeError{Err: err}
	case actionport.IsUnknownOutcome(err):
		return actionport.Effect{ProviderResult: providerResult}, err
	case exchange.Receipt.Document != nil && exchange.Result.Document != nil:
		// A correlated receipt/result pair is a known terminal device
		// response, including a rejected safe stop. Keep it out of the
		// unknown-outcome lane while the safe-stop request remains latched.
		return actionport.Effect{ProviderResult: providerResult}, err
	default:
		return actionport.Effect{ProviderResult: providerResult}, &actionport.UnknownOutcomeError{Err: err}
	}
}

// VerifyDeviceCommand reads one fresh state record and compares the observed
// output with the bounded command materialized from the catalog. The returned
// evidence is suitable for durable unknown-outcome reconciliation.
func (e *GatewayEffector) VerifyDeviceCommand(ctx context.Context, command actionport.Command) (string, map[string]any, error) {
	if e == nil || e.session == nil || e.catalog == nil {
		return "", nil, fmt.Errorf("serial effector session and catalog are required")
	}
	expectedBootID := e.session.BootID()
	wireCommand, err := e.catalog.Materialize(command, expectedBootID)
	if err != nil {
		return "", nil, fmt.Errorf("materialize serial command for verification: %w", err)
	}
	return e.verifyMaterializedCommand(ctx, wireCommand, expectedBootID)
}

func (e *GatewayEffector) verifyMaterializedCommand(ctx context.Context, wireCommand domain.Command, expectedBootID string) (string, map[string]any, error) {
	state, evidence, err := e.session.QueryStateEvidence(ctx, wireCommand.Target)
	if err != nil {
		return "", nil, err
	}
	observedBootID := state.BootID
	if observedBootID != expectedBootID {
		return "", evidence, fmt.Errorf("device boot changed during verification from %q to %q", expectedBootID, observedBootID)
	}
	return verifyObservedOutput(state, evidence, wireCommand)
}

func verifyObservedOutput(state domain.State, evidence map[string]any, wireCommand domain.Command) (string, map[string]any, error) {
	verified, err := domain.OutputVerified(state, wireCommand)
	if err != nil {
		return "", evidence, fmt.Errorf("verify device output: %w", err)
	}
	if !verified {
		return "failed", evidence, nil
	}
	return "succeeded", evidence, nil
}

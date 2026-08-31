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
		if e.session != nil {
			e.session.WithTelemetry(runtimeTelemetry)
		}
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

// SafeStop requests the catalog-owned safe state through the session priority
// lane. It is intentionally separate from policy-approved ordinary dispatch.
func (e *SerialEffector) SafeStop(ctx context.Context, target string) (Effect, error) {
	if e == nil || e.session == nil {
		return Effect{}, fmt.Errorf("serial effector session is required")
	}
	receipt, sent, err := e.session.SafeStop(ctx, target)
	if err != nil {
		if sent {
			return Effect{}, &UnknownOutcomeError{Err: err}
		}
		return Effect{}, err
	}
	return Effect{ProviderResult: map[string]any{"receipt": receipt}, VerificationPending: true}, nil
}

// VerifyDeviceCommand reads one fresh state record and compares the observed
// output with the bounded command materialized from the catalog. The returned
// evidence is suitable for durable unknown-outcome reconciliation.
func (e *SerialEffector) VerifyDeviceCommand(ctx context.Context, command Command) (string, map[string]any, error) {
	if e == nil || e.session == nil || e.catalog == nil {
		return "", nil, fmt.Errorf("serial effector session and catalog are required")
	}
	expectedBootID := e.session.BootID()
	wireCommand, err := e.catalog.Materialize(command, expectedBootID)
	if err != nil {
		return "", nil, fmt.Errorf("materialize serial command for verification: %w", err)
	}
	evidence, err := e.session.QueryStateEvidence(ctx)
	if err != nil {
		return "", nil, err
	}
	observedBootID := documentString(evidence, "boot_id")
	if observedBootID != expectedBootID {
		return "", evidence, fmt.Errorf("device boot changed during verification from %q to %q", expectedBootID, observedBootID)
	}
	if err := setEvidenceTarget(evidence, documentString(wireCommand, "target")); err != nil {
		return "", nil, err
	}
	state, _ := evidence["state"].(map[string]any)
	output, _ := state["current_output"].(map[string]any)
	observedTarget, _ := output["target"].(string)
	observedOperation, _ := output["operation"].(string)
	observedEnergized, _ := output["energized"].(bool)
	expectedValue, expectedEnergized := expectedOutput(wireCommand)
	observedValue, _ := output["value"].(float64)
	valueMatches := true
	if command.EffectorRoute == "set_indicator" {
		valueMatches = observedValue == expectedValue
	}
	if observedTarget != wireCommand["target"] || observedOperation != wireCommand["operation"] || observedEnergized != expectedEnergized || !valueMatches {
		return "failed", evidence, nil
	}
	return "succeeded", evidence, nil
}

func expectedOutput(command map[string]any) (float64, bool) {
	parameters, _ := command["parameters"].(map[string]any)
	for name, raw := range parameters {
		if name == "lease_ms" {
			continue
		}
		value, ok := raw.(float64)
		if ok {
			return value, value > 0
		}
	}
	return 0, false
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

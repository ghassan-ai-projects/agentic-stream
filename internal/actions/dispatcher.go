// Package actions is the governed dispatcher after policy approval: it leases
// approved commands, dispatches them through effect ports, and verifies and
// reconciles their outcomes. Concrete effect adapters live elsewhere.
package actions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// Dispatcher leases approved command outbox rows and records durable results.
type Dispatcher struct {
	db           *storage.DB
	effector     actionport.Effector
	clk          clock.Clock
	idGen        ids.Generator
	owner        string
	leaseFor     time.Duration
	runtimeOwner *runtimecontrol.RuntimeOwner
	runtimeEpoch string
	interlock    interlock.Reader
	telemetry    *telemetry.Runtime
}

// WithRuntimeOwner fences dispatcher ledger mutations to the active runtime
// lease. It returns the dispatcher for composition during startup.
func (d *Dispatcher) WithRuntimeOwner(owner *runtimecontrol.RuntimeOwner, epoch string) *Dispatcher {
	d.runtimeOwner = owner
	d.runtimeEpoch = epoch
	return d
}

// WithInterlock adds the final read-only readiness check before effect delivery.
func (d *Dispatcher) WithInterlock(reader interlock.Reader) *Dispatcher {
	d.interlock = reader
	return d
}

// WithTelemetry connects action-dispatch counters to the runtime telemetry
// surface. It is optional for embedders and tests.
func (d *Dispatcher) WithTelemetry(runtimeTelemetry *telemetry.Runtime) *Dispatcher {
	if d != nil {
		d.telemetry = runtimeTelemetry
	}
	return d
}

// NewDispatcher creates an action dispatcher. leaseFor controls how long an
// abandoned lease remains protected from another dispatcher.
func NewDispatcher(db *storage.DB, effector actionport.Effector, clk clock.Clock, idGen ids.Generator, owner string, leaseFor time.Duration) *Dispatcher {
	if clk == nil {
		clk = clock.Physical()
	}
	if idGen == nil {
		idGen = ids.Random()
	}
	if owner == "" {
		owner = "actions"
	}
	if leaseFor <= 0 {
		leaseFor = time.Minute
	}
	return &Dispatcher{db: db, effector: effector, clk: clk, idGen: idGen, owner: owner, leaseFor: leaseFor}
}

type leasedCommand struct {
	OutboxID    int64
	Command     actionport.Command
	CommandJSON []byte
	CommandSHA  []byte
	LeaseOwner  string
	Traceparent string
	Tracestate  string
}

// DispatchOnce processes at most one command. A leased command is marked
// dispatching before the provider call, and all ledger changes are committed
// atomically after the provider returns.
func (d *Dispatcher) DispatchOnce(ctx context.Context) (bool, error) {
	leased, found, err := d.lease(ctx)
	if err != nil || !found {
		return found, err
	}
	if d.effector == nil {
		return true, d.finalize(ctx, leased, actionport.Effect{}, errors.New("no effector configured"))
	}
	if err := d.revalidateAuthorization(ctx, leased); err != nil {
		return true, d.finalize(ctx, leased, actionport.Effect{}, fmt.Errorf("authorization revalidation failed: %w", err))
	}
	callCtx, cancel := d.dispatchContext(ctx)
	defer cancel()
	return true, d.dispatchLeasedCommand(ctx, callCtx, leased)
}

func (d *Dispatcher) dispatchContext(ctx context.Context) (context.Context, context.CancelFunc) {
	callTimeout := d.leaseFor - d.leaseFor/10
	if callTimeout <= 0 {
		callTimeout = d.leaseFor
	}
	return context.WithTimeout(ctx, callTimeout)
}

func (d *Dispatcher) dispatchLeasedCommand(ctx, callCtx context.Context, leased leasedCommand) error {
	if d.interlock != nil {
		guarded, ok := d.effector.(actionport.AuthorizedEffector)
		if !ok {
			return d.finalize(ctx, leased, actionport.Effect{}, errors.New("configured effector does not enforce dispatch authorization"))
		}
		effect, dispatchErr := guarded.DispatchAuthorized(callCtx, leased.Command, runtimecontrol.NewDispatchAuthorization(d.db, d.interlock, leased.Command.TenantID, leased.Command.NormalizedTarget))
		if errors.Is(dispatchErr, context.DeadlineExceeded) {
			dispatchErr = &actionport.UnknownOutcomeError{Err: dispatchErr}
		}
		return d.finalizeDispatch(ctx, callCtx, leased, effect, dispatchErr)
	}
	effect, dispatchErr := d.effector.Dispatch(callCtx, leased.Command)
	if errors.Is(dispatchErr, context.DeadlineExceeded) {
		dispatchErr = &actionport.UnknownOutcomeError{Err: dispatchErr}
	}
	return d.finalizeDispatch(ctx, callCtx, leased, effect, dispatchErr)
}

func (d *Dispatcher) finalizeDispatch(ctx, verifyCtx context.Context, leased leasedCommand, effect actionport.Effect, dispatchErr error) error {
	check := d.verifyDevice(verifyCtx, leased, effect, dispatchErr)
	if err := d.finalize(ctx, leased, check.effect, check.dispatchErr); err != nil {
		return err
	}
	if !check.reconcilesUnknown() {
		return nil
	}
	return d.ReconcileUnknown(ctx, leased.Command.CommandID, check.finalStatus, check.evidence)
}

// deviceCheck is a dispatch result after independent device-state
// verification.
type deviceCheck struct {
	effect      actionport.Effect
	dispatchErr error
	finalStatus string
	evidence    map[string]any
	verifyErr   error
}

// reconcilesUnknown reports whether verification settled an outcome the
// transport left unknown.
func (c deviceCheck) reconcilesUnknown() bool {
	return actionport.IsUnknownOutcome(c.dispatchErr) && c.finalStatus != "" && c.verifyErr == nil
}

// verifyDevice reads the device state after a successful or unknown dispatch
// when the effector can verify it. A verification error makes a successful
// dispatch unknown, and a failed verification makes it failed.
func (d *Dispatcher) verifyDevice(ctx context.Context, leased leasedCommand, effect actionport.Effect, dispatchErr error) deviceCheck {
	check := deviceCheck{effect: effect, dispatchErr: dispatchErr}
	verifier, canVerify := d.effector.(actionport.DeviceStateVerifier)
	if !canVerify || (dispatchErr != nil && !actionport.IsUnknownOutcome(dispatchErr)) {
		return check
	}
	check.finalStatus, check.evidence, check.verifyErr = verifier.VerifyDeviceCommand(ctx, leased.Command)
	if check.finalStatus != "" {
		check.effect.ObservedEffect = check.evidence
	}
	if dispatchErr != nil {
		return check
	}
	switch {
	case check.verifyErr != nil:
		check.effect.VerificationPending = false
		check.dispatchErr = &actionport.UnknownOutcomeError{Err: fmt.Errorf("verify device state: %w", check.verifyErr)}
	case check.finalStatus != "":
		check.effect.VerificationPending = false
		if check.finalStatus == "failed" {
			check.dispatchErr = errors.New("device state verification failed")
		}
	}
	return check
}

func (d *Dispatcher) assertRuntimeOwner(ctx context.Context, tx *sql.Tx) error {
	if d.runtimeOwner == nil || d.runtimeEpoch == "" {
		return nil
	}
	if err := d.runtimeOwner.Assert(ctx, tx, d.runtimeEpoch); err != nil {
		return fmt.Errorf("action runtime ownership lost: %w", err)
	}
	return nil
}

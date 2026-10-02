// Package actions owns the effect boundary after policy approval.
package actions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// Command is the validated, policy-approved input to an effector.
type Command struct {
	CommandID        string
	IntentID         string
	TenantID         string
	EffectorRoute    string
	NormalizedTarget string
	IdempotencyKey   string
	PolicyDigest     string
	NotBeforeMonoUS  int64
	Payload          map[string]any
}

// Effect is the provider response. An effector must return UnknownOutcomeError
// when it cannot establish whether the provider applied the effect. Set
// VerificationPending when the provider only acknowledged transport receipt;
// independent feedback must then establish physical success.
type Effect struct {
	ProviderResult      map[string]any
	ObservedEffect      map[string]any
	VerificationPending bool
}

// Effector is the only interface allowed to cross from the action plane into
// an external system. Implementations must honor Command.IdempotencyKey.
type Effector interface {
	Dispatch(context.Context, Command) (Effect, error)
}

// DeviceStateVerifier verifies a device-backed command with one fresh state
// query. An empty final status means the command is not device-backed.
type DeviceStateVerifier interface {
	VerifyDeviceCommand(context.Context, Command) (finalStatus string, evidence map[string]any, err error)
}

// Authorization is the final runtime authorization check passed to a
// concrete effector. The check must run immediately before the effect is
// accepted by that effector.
type Authorization struct {
	Check func(context.Context) error
}

// AuthorizedEffector is required when the dispatcher has a live interlock.
// It closes the validation-to-acceptance gap at the concrete effect boundary.
type AuthorizedEffector interface {
	Effector
	DispatchAuthorized(context.Context, Command, Authorization) (Effect, error)
}

// UnknownOutcomeError means the request may have reached the provider, so the
// dispatcher records reconciliation as required and never blindly retries it.
type UnknownOutcomeError struct{ Err error }

func (e *UnknownOutcomeError) Error() string {
	if e == nil || e.Err == nil {
		return "action outcome is unknown"
	}
	return "action outcome is unknown: " + e.Err.Error()
}

func (e *UnknownOutcomeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsUnknownOutcome reports whether err requires reconciliation instead of an
// automatic retry.
func IsUnknownOutcome(err error) bool {
	var unknown *UnknownOutcomeError
	return errors.As(err, &unknown)
}

// Dispatcher leases approved command outbox rows and records durable results.
type Dispatcher struct {
	db           *storage.DB
	effector     Effector
	clk          clock.Clock
	idGen        ids.Generator
	owner        string
	leaseFor     time.Duration
	runtimeOwner *storage.RuntimeOwner
	runtimeEpoch string
	interlock    interlock.Reader
	telemetry    *telemetry.Runtime
}

// WithRuntimeOwner fences dispatcher ledger mutations to the active runtime
// lease. It returns the dispatcher for composition during startup.
func (d *Dispatcher) WithRuntimeOwner(owner *storage.RuntimeOwner, epoch string) *Dispatcher {
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
func NewDispatcher(db *storage.DB, effector Effector, clk clock.Clock, idGen ids.Generator, owner string, leaseFor time.Duration) *Dispatcher {
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
	Command     Command
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
		return true, d.finalize(ctx, leased, Effect{}, errors.New("no effector configured"))
	}
	if err := d.revalidateAuthorization(ctx, leased); err != nil {
		return true, d.finalize(ctx, leased, Effect{}, fmt.Errorf("authorization revalidation failed: %w", err))
	}
	callTimeout := d.leaseFor - d.leaseFor/10
	if callTimeout <= 0 {
		callTimeout = d.leaseFor
	}
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	if d.interlock != nil {
		guarded, ok := d.effector.(AuthorizedEffector)
		if !ok {
			return true, d.finalize(ctx, leased, Effect{}, errors.New("configured effector does not enforce dispatch authorization"))
		}
		effect, dispatchErr := guarded.DispatchAuthorized(callCtx, leased.Command, Authorization{Check: func(checkCtx context.Context) error {
			return d.assertInterlock(checkCtx, leased.Command)
		}})
		if errors.Is(dispatchErr, context.DeadlineExceeded) {
			dispatchErr = &UnknownOutcomeError{Err: dispatchErr}
		}
		return true, d.finalizeDispatch(ctx, callCtx, leased, effect, dispatchErr)
	}
	effect, dispatchErr := d.effector.Dispatch(callCtx, leased.Command)
	if errors.Is(dispatchErr, context.DeadlineExceeded) {
		dispatchErr = &UnknownOutcomeError{Err: dispatchErr}
	}
	return true, d.finalizeDispatch(ctx, callCtx, leased, effect, dispatchErr)
}

func (d *Dispatcher) finalizeDispatch(ctx, verifyCtx context.Context, leased leasedCommand, effect Effect, dispatchErr error) error {
	verifier, canVerify := d.effector.(DeviceStateVerifier)
	finalStatus := ""
	var evidence map[string]any
	var verifyErr error
	if canVerify && (dispatchErr == nil || IsUnknownOutcome(dispatchErr)) {
		finalStatus, evidence, verifyErr = verifier.VerifyDeviceCommand(verifyCtx, leased.Command)
		if finalStatus != "" {
			effect.ObservedEffect = evidence
		}
		if dispatchErr == nil && verifyErr != nil {
			effect.VerificationPending = false
			dispatchErr = &UnknownOutcomeError{Err: fmt.Errorf("verify device state: %w", verifyErr)}
		} else if dispatchErr == nil && finalStatus != "" {
			effect.VerificationPending = false
			if finalStatus == "failed" {
				dispatchErr = errors.New("device state verification failed")
			}
		}
	}
	if err := d.finalize(ctx, leased, effect, dispatchErr); err != nil {
		return err
	}
	if IsUnknownOutcome(dispatchErr) && finalStatus != "" && verifyErr == nil {
		if err := d.ReconcileUnknown(ctx, leased.Command.CommandID, finalStatus, evidence); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) assertInterlock(ctx context.Context, command Command) error {
	if err := d.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := d.interlock.Assert(ctx, tx, command.TenantID, command.NormalizedTarget, ""); err != nil {
			return fmt.Errorf("dispatch interlock assertion: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("assert dispatch interlock: %w", err)
	}
	return nil
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

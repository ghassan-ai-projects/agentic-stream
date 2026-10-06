package domain

import (
	"context"
	"errors"
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

// VerifiedEffector combines final authorization and independently observed device state.
type VerifiedEffector interface {
	AuthorizedEffector
	DeviceStateVerifier
}

package actionport

import (
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/actionport/internal/domain"
)

// Command is the validated, policy-approved input to an effector.
type Command = domain.Command

// Effect is the provider response. An effector must return UnknownOutcomeError
// when it cannot establish whether the provider applied the effect. Set
// VerificationPending when the provider only acknowledged transport receipt;
// independent feedback must then establish physical success.
type Effect = domain.Effect

// Effector is the only interface allowed to cross from the action plane into
// an external system. Implementations must honor Command.IdempotencyKey.
type Effector = domain.Effector

// DeviceStateVerifier verifies a device-backed command with one fresh state
// query. An empty final status means the command is not device-backed.
type DeviceStateVerifier = domain.DeviceStateVerifier

// Authorization is the final runtime authorization check passed to a
// concrete effector. The check must run immediately before the effect is
// accepted by that effector.
type Authorization = domain.Authorization

// AuthorizedEffector is required when the dispatcher has a live interlock.
// It closes the validation-to-acceptance gap at the concrete effect boundary.
type AuthorizedEffector = domain.AuthorizedEffector

// UnknownOutcomeError means the request may have reached the provider, so the
// dispatcher records reconciliation as required and never blindly retries it.
type UnknownOutcomeError = domain.UnknownOutcomeError

// IsUnknownOutcome reports whether err requires reconciliation instead of an
// automatic retry.
func IsUnknownOutcome(err error) bool {
	return domain.IsUnknownOutcome(err)
}

// VerifiedEffector combines final authorization and independently observed device state.
type VerifiedEffector = domain.VerifiedEffector

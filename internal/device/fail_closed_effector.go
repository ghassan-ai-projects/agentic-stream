package device

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
)

// FailClosedEffector rejects routes that are not explicitly owned by another
// configured effector. It prevents a live device profile from silently
// converting an unsupported route into a simulated success.
type FailClosedEffector struct {
	profile EffectProfile
}

// NewFailClosedEffector creates the fallback for live profiles whose
// non-device routes must not be simulated.
func NewFailClosedEffector(profile EffectProfile) *FailClosedEffector {
	return &FailClosedEffector{profile: profile}
}

// Dispatch rejects a route that has no live effector mapping.
func (e *FailClosedEffector) Dispatch(ctx context.Context, command actionport.Command) (actionport.Effect, error) {
	if err := ctx.Err(); err != nil {
		return actionport.Effect{}, fmt.Errorf("%s fallback dispatch canceled: %w", e.profile, err)
	}
	return actionport.Effect{}, fmt.Errorf("%s effect profile has no effector for route %q", e.profile, command.EffectorRoute)
}

// DispatchAuthorized preserves the final interlock check before rejecting the
// unmapped route.
func (e *FailClosedEffector) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	if authorization.Check == nil {
		return actionport.Effect{}, fmt.Errorf("dispatch authorization is required")
	}
	if err := authorization.Check(ctx); err != nil {
		return actionport.Effect{}, fmt.Errorf("%w", err)
	}
	return e.Dispatch(ctx, command)
}

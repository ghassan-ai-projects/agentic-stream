package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
)

func dispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization, dispatch func(context.Context, actionport.Command) (actionport.Effect, error)) (actionport.Effect, error) {
	if err := authorizeDispatch(ctx, authorization); err != nil {
		return actionport.Effect{}, err
	}
	return dispatch(ctx, command)
}

// authorizeDispatch checks the final interlock before any effector work.
func authorizeDispatch(ctx context.Context, authorization actionport.Authorization) error {
	if authorization.Check == nil {
		return fmt.Errorf("dispatch authorization is required")
	}
	if err := authorization.Check(ctx); err != nil {
		return fmt.Errorf("dispatch authorization: %w", err)
	}
	return nil
}

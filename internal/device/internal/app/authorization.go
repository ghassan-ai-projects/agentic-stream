package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
)

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

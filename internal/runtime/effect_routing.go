package runtime

import (
	"context"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	app "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

// CompositeEffector preserves governed routing to watch, device and fallback effects.
type CompositeEffector struct{ application *app.CompositeEffector }

// NewCompositeEffector constructs the action routing facade.
func NewCompositeEffector(w *watch.Effector, fallback actionport.Effector) *CompositeEffector {
	return &CompositeEffector{application: app.NewCompositeEffector(w, fallback)}
}
func (e *CompositeEffector) useCases() *app.CompositeEffector {
	if e == nil {
		return nil
	}
	return e.application
}

// WithSerial binds the closed device routes.
func (e *CompositeEffector) WithSerial(serial actionport.VerifiedEffector) *CompositeEffector {
	e.useCases().WithSerial(serial)
	return e
}

// Dispatch delegates one governed route.
func (e *CompositeEffector) Dispatch(ctx context.Context, c actionport.Command) (actionport.Effect, error) {
	return e.useCases().Dispatch(ctx, c)
}

// DispatchAuthorized retains final authorization at the selected effector.
func (e *CompositeEffector) DispatchAuthorized(ctx context.Context, c actionport.Command, a actionport.Authorization) (actionport.Effect, error) {
	return e.useCases().DispatchAuthorized(ctx, c, a)
}

// VerifyDeviceCommand delegates verification for the closed device routes.
func (e *CompositeEffector) VerifyDeviceCommand(ctx context.Context, c actionport.Command) (string, map[string]any, error) {
	return e.useCases().VerifyDeviceCommand(ctx, c)
}

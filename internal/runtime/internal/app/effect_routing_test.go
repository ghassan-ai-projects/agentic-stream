package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
)

type guardedRoute struct{ calls int }

func (r *guardedRoute) Dispatch(context.Context, actionport.Command) (actionport.Effect, error) {
	r.calls++
	return actionport.Effect{}, nil
}
func (r *guardedRoute) DispatchAuthorized(ctx context.Context, command actionport.Command, auth actionport.Authorization) (actionport.Effect, error) {
	if err := auth.Check(ctx); err != nil {
		return actionport.Effect{}, err
	}
	return r.Dispatch(ctx, command)
}
func (*guardedRoute) VerifyDeviceCommand(context.Context, actionport.Command) (string, map[string]any, error) {
	return "succeeded", map[string]any{"verified": true}, nil
}

func TestEffectRoutingPreservesAuthorizationAndCannotFallbackDeviceRoutes(t *testing.T) {
	t.Parallel()
	fallback, physical := &guardedRoute{}, &guardedRoute{}
	routes := app.NewCompositeEffector(nil, fallback)
	for _, route := range []string{"set_indicator", "select_thermal_mode"} {
		if _, err := routes.Dispatch(t.Context(), actionport.Command{EffectorRoute: route}); err == nil {
			t.Fatalf("missing device route %s fell through to fallback", route)
		}
	}
	if fallback.calls != 0 {
		t.Fatal("device routes reached fallback")
	}
	routes.WithSerial(physical)
	denied := errors.New("interlock denied")
	checks := 0
	_, err := routes.DispatchAuthorized(t.Context(), actionport.Command{EffectorRoute: "set_indicator"}, actionport.Authorization{Check: func(context.Context) error { checks++; return denied }})
	if !errors.Is(err, denied) || checks != 1 || physical.calls != 0 {
		t.Fatalf("authorization: err=%v checks=%d effects=%d", err, checks, physical.calls)
	}
	_, err = routes.DispatchAuthorized(t.Context(), actionport.Command{EffectorRoute: "set_indicator"}, actionport.Authorization{})
	if err == nil || physical.calls != 0 {
		t.Fatal("missing authorization reached effect")
	}
	status, _, err := routes.VerifyDeviceCommand(t.Context(), actionport.Command{EffectorRoute: "set_indicator"})
	if err != nil || status != "succeeded" {
		t.Fatalf("verification route: %s %v", status, err)
	}
}

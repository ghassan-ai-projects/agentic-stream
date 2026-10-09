package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
)

func TestEveryEffectorChecksAuthorizationBeforeDispatch(t *testing.T) {
	t.Parallel()
	denied := errors.New("interlock denied")
	effectors := map[string]actionport.AuthorizedEffector{
		"gateway":     app.NewGatewayEffector(nil, nil),
		"simulated":   app.NewSimulatedEffector(),
		"fail closed": app.NewFailClosedEffector(domain.EffectProfilePhysical),
	}
	for name, effector := range effectors {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err := effector.DispatchAuthorized(ctx, actionport.Command{}, actionport.Authorization{})
			if err == nil || err.Error() != "dispatch authorization is required" {
				t.Fatalf("missing authorization lost precedence: %v", err)
			}
			checks := 0
			effect, err := effector.DispatchAuthorized(ctx, actionport.Command{}, actionport.Authorization{Check: func(context.Context) error {
				checks++
				return denied
			}})
			if checks != 1 || !errors.Is(err, denied) || effect.ProviderResult != nil || effect.ObservedEffect != nil {
				t.Fatalf("checks=%d, effect=%+v, error=%v", checks, effect, err)
			}
		})
	}
}

func TestFailClosedEffectorRefusesEveryRouteItDoesNotOwn(t *testing.T) {
	t.Parallel()
	effector := app.NewFailClosedEffector(domain.EffectProfilePhysical)
	_, err := effector.Dispatch(t.Context(), actionport.Command{EffectorRoute: "start_aerator"})
	if err == nil || err.Error() != `physical effect profile has no effector for route "start_aerator"` {
		t.Fatalf("unmapped route error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := effector.Dispatch(ctx, actionport.Command{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled dispatch error = %v, want context.Canceled", err)
	}
}

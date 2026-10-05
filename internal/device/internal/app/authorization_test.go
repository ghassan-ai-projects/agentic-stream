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
	for name, effector := range map[string]actionport.AuthorizedEffector{
		"gateway":     app.NewGatewayEffector(nil, nil),
		"simulated":   app.NewSimulatedEffector(),
		"fail closed": app.NewFailClosedEffector(domain.EffectProfilePhysical),
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			command := actionport.Command{}
			if _, err := effector.DispatchAuthorized(ctx, command, actionport.Authorization{}); err == nil || err.Error() != "dispatch authorization is required" {
				t.Fatalf("missing authorization lost precedence: %v", err)
			}
			checks := 0
			effect, err := effector.DispatchAuthorized(ctx, command, actionport.Authorization{Check: func(context.Context) error {
				checks++
				return denied
			}})
			if checks != 1 || !errors.Is(err, denied) || effect.ProviderResult != nil || effect.ObservedEffect != nil {
				t.Fatalf("checks=%d, effect=%+v, error=%v", checks, effect, err)
			}
		})
	}
}

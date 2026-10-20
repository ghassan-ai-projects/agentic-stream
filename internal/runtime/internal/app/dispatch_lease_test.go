package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
)

type heldEffector struct{}

func (heldEffector) OwnsRoute(string) bool { return true }

func (heldEffector) Dispatch(ctx context.Context, _ actionport.Command) (actionport.Effect, error) {
	<-ctx.Done()
	return actionport.Effect{}, ctx.Err()
}

func (e heldEffector) DispatchAuthorized(ctx context.Context, command actionport.Command, _ actionport.Authorization) (actionport.Effect, error) {
	return e.Dispatch(ctx, command)
}

func TestTheDispatchLeaseBoundsACommandHeldAtTheDevice(t *testing.T) {
	t.Parallel()
	pipeline, db := openPipeline(t, thingSpec("native", "active"), runtime.PipelineConfig{Effector: heldEffector{}, DispatchLease: 200 * time.Millisecond})
	if err := pipeline.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, shutdown := context.WithCancel(t.Context())
	defer shutdown()
	go func() {
		_ = pipeline.RunFeed(ctx, func(ctx context.Context, emit func(context.Context, contractsv1.Envelope) error) error {
			if err := emit(ctx, liveEnvelope("evt-held", "thing-1", 15)); err != nil {
				return err
			}
			<-ctx.Done()
			return nil
		})
	}()
	waitUntil(t, "the held command to end as an unknown outcome", func() bool {
		return scalar[int](t, db, "SELECT COUNT(*) FROM outcomes WHERE status = 'unknown'") == 1
	})
}

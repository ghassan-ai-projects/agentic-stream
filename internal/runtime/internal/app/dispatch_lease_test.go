package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
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
	go func() { _ = pipeline.RunFeed(ctx, feedOne(liveEnvelope("evt-held", "thing-1", 15))) }()
	waitUntil(t, "the held command to end as an unknown outcome", func() bool {
		return scalar[int](t, db, "SELECT COUNT(*) FROM outcomes WHERE status = 'unknown'") == 1
	})
}

type recordingDevice struct {
	heldEffector
	commands chan string
}

func (d recordingDevice) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	if err := authorization.Check(ctx); err != nil {
		return actionport.Effect{}, err
	}
	d.commands <- command.CommandID
	return actionport.Effect{ProviderResult: map[string]any{"device": "recorded"}}, nil
}

func (recordingDevice) VerifyDeviceCommand(context.Context, actionport.Command) (string, map[string]any, error) {
	return "", nil, nil
}

func TestADeviceEffectorGivenByInterfaceOwnsItsRoutes(t *testing.T) {
	t.Parallel()
	recorder := recordingDevice{commands: make(chan string, 1)}
	pipeline, db := openPipeline(t, thingSpec("native", "active"), runtime.PipelineConfig{DeviceEffector: recorder})
	if err := pipeline.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, shutdown := context.WithCancel(t.Context())
	defer shutdown()
	go func() { _ = pipeline.RunFeed(ctx, feedOne(liveEnvelope("evt-device", "thing-1", 15))) }()
	select {
	case <-recorder.commands:
	case <-time.After(10 * time.Second):
		t.Fatal("the device effector never received the command")
	}
	waitUntil(t, "the device command to be recorded", func() bool {
		return scalar[int](t, db, "SELECT COUNT(*) FROM commands WHERE status = 'succeeded'") == 1
	})
}

func TestAPipelineTakesOneDeviceEffectLink(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	_, err := runtime.NewPipeline(t.Context(), runtime.PipelineConfig{DB: db, Spec: thingSpec("native", "active"),
		Effector: heldEffector{}, GatewayEffector: &device.GatewayEffector{}, DeviceEffector: recordingDevice{}})
	if err == nil || !strings.Contains(err.Error(), "one device effect link per pipeline") {
		t.Fatalf("NewPipeline with two device links = %v, want a refusal", err)
	}
}

func feedOne(event contractsv1.Envelope) runtime.Feed {
	return func(ctx context.Context, emit func(context.Context, contractsv1.Envelope) error) error {
		if err := emit(ctx, event); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	}
}

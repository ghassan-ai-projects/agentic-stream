package composition

import (
	"context"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	app "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

// PipelineConfig configures one owner-scoped live runtime pipeline.
type PipelineConfig struct {
	DB                *storage.DB
	Spec              *spec.CompiledSpec
	TenantID          string
	Owner             *runtimecontrol.RuntimeOwner
	OwnerEpoch        string
	Clock             clock.Clock
	Executor          episodes.Executor
	Effector          actionport.Effector
	IDGenerator       ids.Generator
	GlobalCostCeiling *uint64
	TenantCostCeiling *uint64
	CostKillSwitch    *bool
	Telemetry         *telemetry.Runtime
	// P8: DemoMode admits `fixture` executors (demos and tests only). A
	// production pipeline (DemoMode false) rejects them at admission.
	DemoMode bool
	// P8: the epoch-control reader — nil in tests without drain/kill. When
	// set, admission refuses new episodes while the epoch is draining and
	// every later decision is refused once the epoch is killed.
	EpochControl *runtimecontrol.EpochControl
	// GatewayEffector is optional and supplies the explicitly routed thermal
	// action boundary. It is never used by replay or shadow execution.
	GatewayEffector *device.GatewayEffector
}

// NewPipeline creates a fully composed live pipeline. The caller must start
// the runtime Service first when Owner is configured.
func NewPipeline(ctx context.Context, cfg PipelineConfig) (*app.Pipeline, error) {
	if cfg.DB == nil || cfg.Spec == nil {
		return nil, fmt.Errorf("pipeline database and spec are required")
	}
	cfg = pipelineDefaults(cfg)
	effector, watch, err := composeEffectors(cfg)
	if err != nil {
		return nil, err
	}
	cfg.Effector = effector
	return composeOwnedPipeline(ctx, cfg, watch)
}

func composeOwnedPipeline(ctx context.Context, cfg PipelineConfig, watch *watch.Service) (*app.Pipeline, error) {
	log := eventlog.NewEventLogWithClock(cfg.DB, cfg.Clock)
	stream, err := newOwnedStream(ctx, cfg, log)
	if err != nil {
		return nil, err
	}
	if err := configureCostLimits(ctx, cfg); err != nil {
		return nil, err
	}
	return composePipeline(cfg, log, stream, watch)
}

func newOwnedStream(ctx context.Context, cfg PipelineConfig, log *eventlog.EventLog) (*engine.Service, error) {
	stream, err := engine.New(ctx, engine.Config{DB: cfg.DB, Log: log, Clock: cfg.Clock, Spec: cfg.Spec, TenantID: cfg.TenantID,
		RuntimeOwner: runtimeOwnershipCheck(cfg), Epoch: cfg.OwnerEpoch, Cognition: true})
	if err != nil {
		return nil, fmt.Errorf("create stream engine: %w", err)
	}
	return stream, nil
}

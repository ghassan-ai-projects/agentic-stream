package runtime

import (
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/admission"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
)

func pipelineDefaults(cfg PipelineConfig) PipelineConfig {
	if cfg.TenantID == "" {
		cfg.TenantID = "default"
	}
	if cfg.Clock == nil {
		cfg.Clock = clock.Physical()
	}
	if cfg.IDGenerator == nil {
		cfg.IDGenerator = ids.Random()
	}
	if cfg.Executor == nil {
		cfg.Executor = episodes.NewFakeExecutor()
	}
	if cfg.Effector == nil {
		cfg.Effector = actions.NewSimulatedEffector()
	}
	return cfg
}

func composeEffectors(cfg PipelineConfig) (actionport.Effector, *actions.WatchEffector) {
	watch := actions.NewWatchEffectorWithClock(cfg.DB, cfg.Clock)
	watch.WithRuntimeOwner(cfg.Owner, cfg.OwnerEpoch)
	watch.WithInterlock(interlock.DurableReader{})
	serialEffector := cfg.SerialEffector
	if serialEffector != nil {
		serialEffector.WithTelemetry(cfg.Telemetry)
	}
	compositeEffector := NewCompositeEffector(watch, cfg.Effector)
	if serialEffector != nil {
		compositeEffector.WithSerial(serialEffector)
	}
	return compositeEffector, watch
}

func composePipeline(cfg PipelineConfig, log *eventlog.EventLog, stream *engine.Engine, watch *actions.WatchEffector) *Pipeline {
	assembler, runner := composeCognition(cfg)
	admitter := admission.New(admission.Config{
		DB: cfg.DB, Assembler: assembler, Clock: cfg.Clock, TenantID: cfg.TenantID,
		Owner: cfg.Owner, OwnerEpoch: cfg.OwnerEpoch, EpochControl: cfg.EpochControl, DemoMode: cfg.DemoMode,
	})
	policyGateway := composePolicy(cfg)
	dispatcher := composeDispatcher(cfg)
	return &Pipeline{
		db:         cfg.DB,
		log:        log,
		engine:     stream,
		admission:  admitter,
		runner:     runner,
		policy:     policyGateway,
		dispatcher: dispatcher,
		watch:      watch,
		telemetry:  cfg.Telemetry,
		owner:      cfg.Owner,
		ownerEpoch: cfg.OwnerEpoch,
		clk:        cfg.Clock,
		tenantID:   cfg.TenantID,
	}
}

func composeCognition(cfg PipelineConfig) (*episodes.Assembler, *episodes.Runner) {
	assembler := episodes.NewAssembler(cfg.Spec, cfg.IDGenerator)
	assembler.WithCostControl(&costcontrol.Controller{})
	runner := episodes.NewRunnerWithEpoch(cfg.DB, cfg.Executor, cfg.Clock, cfg.IDGenerator, cfg.OwnerEpoch)
	runner.WithAssembler(assembler)
	runner.WithCostControl(&costcontrol.Controller{})
	runner.WithEpochControl(cfg.EpochControl)
	runner.WithShadowStore(&qualification.ShadowStore{DB: cfg.DB})
	runner.WithTelemetry(cfg.Telemetry)
	return assembler, runner
}

func composePolicy(cfg PipelineConfig) *policy.Gateway {
	policyGateway := policy.NewGatewayWithOwner(cfg.Spec.Digest, cfg.IDGenerator, cfg.Owner, cfg.OwnerEpoch)
	policyGateway.WithInterlock(interlock.DurableReader{})
	policyGateway.WithCalibration(&qualification.CalibrationStore{DB: cfg.DB})
	policyGateway.WithEpochControl(cfg.EpochControl)
	return policyGateway
}

func composeDispatcher(cfg PipelineConfig) *actions.Dispatcher {
	dispatcher := actions.NewDispatcher(cfg.DB, cfg.Effector, cfg.Clock, cfg.IDGenerator, "runtime-actions/"+cfg.OwnerEpoch, time.Minute)
	dispatcher.WithRuntimeOwner(cfg.Owner, cfg.OwnerEpoch)
	dispatcher.WithInterlock(interlock.DurableReader{})
	dispatcher.WithTelemetry(cfg.Telemetry)
	return dispatcher
}

package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/admission"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
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
	return pipelineExecutionDefaults(cfg)
}

func pipelineExecutionDefaults(cfg PipelineConfig) PipelineConfig {
	if cfg.Executor == nil {
		cfg.Executor = episodes.NewFakeExecutor()
	}
	if cfg.Effector == nil {
		cfg.Effector = device.NewSimulatedEffector()
	}
	return cfg
}

func composeEffectors(cfg PipelineConfig) (actionport.Effector, *watch.Effector) {
	watch := watch.NewEffectorWithClock(cfg.DB, cfg.Clock)
	watch.WithRuntimeOwner(cfg.Owner, cfg.OwnerEpoch)
	watch.WithInterlock(interlock.DurableReader{})
	gatewayEffector := cfg.GatewayEffector
	if gatewayEffector != nil {
		gatewayEffector.WithTelemetry(cfg.Telemetry)
	}
	compositeEffector := NewCompositeEffector(watch, cfg.Effector)
	if gatewayEffector != nil {
		compositeEffector.WithSerial(gatewayEffector)
	}
	return compositeEffector, watch
}

func composePipeline(cfg PipelineConfig, log *eventlog.EventLog, stream *engine.Engine, watch *watch.Effector) (*Pipeline, error) {
	assembler, runner := composeCognition(cfg)
	admitter := admission.New(admission.Config{
		DB: cfg.DB, Assembler: assembler, Clock: cfg.Clock, TenantID: cfg.TenantID,
		Owner: cfg.Owner, OwnerEpoch: cfg.OwnerEpoch, EpochControl: cfg.EpochControl, DemoMode: cfg.DemoMode,
	})
	policyGateway, err := composePolicy(cfg)
	if err != nil {
		return nil, err
	}
	dispatcher := composeDispatcher(cfg)
	return &Pipeline{
		db: cfg.DB, log: log, engine: stream, admission: admitter, runner: runner,
		policy: policyGateway, dispatcher: dispatcher, watch: watch, telemetry: cfg.Telemetry,
		owner: cfg.Owner, ownerEpoch: cfg.OwnerEpoch, clk: cfg.Clock, tenantID: cfg.TenantID,
	}, nil
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

func composePolicy(cfg PipelineConfig) (*policy.Service, error) {
	service, err := policy.New(policy.Config{PolicyVersion: cfg.Spec.Digest, IDGenerator: cfg.IDGenerator, OwnerEpoch: cfg.OwnerEpoch, RuntimeOwner: policyOwnershipCheck(cfg), DecisionEpoch: policyEpochCheck(cfg), Interlock: interlock.DurableReader{}, Calibration: policyCalibrationCheck(cfg)})
	if err != nil {
		return nil, fmt.Errorf("compose policy: %w", err)
	}
	return service, nil
}

func policyCalibrationCheck(cfg PipelineConfig) policy.CalibrationCheck {
	calibration := &qualification.CalibrationStore{DB: cfg.DB}
	return func(ctx context.Context, tx *sql.Tx, situationType, executorVersion string) error {
		if err := calibration.AssertCalibration(ctx, tx, qualification.CalibrationArtifact{Domain: situationType, ModelRevision: executorVersion}); err != nil {
			return fmt.Errorf("check policy calibration: %w", err)
		}
		return nil
	}
}

func policyOwnershipCheck(cfg PipelineConfig) func(context.Context, *sql.Tx, string) error {
	if cfg.Owner == nil || cfg.OwnerEpoch == "" {
		return unownedPolicyCheck
	}
	return cfg.Owner.Assert
}

func policyEpochCheck(cfg PipelineConfig) func(context.Context, *sql.Tx, string) error {
	if cfg.EpochControl == nil {
		return unownedPolicyCheck
	}
	return cfg.EpochControl.AssertDecisionTx
}

func unownedPolicyCheck(context.Context, *sql.Tx, string) error { return nil }

func composeDispatcher(cfg PipelineConfig) *actions.Dispatcher {
	dispatcher := actions.NewDispatcher(cfg.DB, cfg.Effector, cfg.Clock, cfg.IDGenerator, "runtime-actions/"+cfg.OwnerEpoch, time.Minute)
	dispatcher.WithRuntimeOwner(cfg.Owner, cfg.OwnerEpoch)
	dispatcher.WithInterlock(interlock.DurableReader{})
	dispatcher.WithTelemetry(cfg.Telemetry)
	return dispatcher
}

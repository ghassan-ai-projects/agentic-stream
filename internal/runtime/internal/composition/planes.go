package composition

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
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
	app "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/transport"
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
		cfg.Executor = fixture.New()
	}
	if cfg.Effector == nil {
		cfg.Effector = device.NewSimulatedEffector()
	}
	return cfg
}

func composeEffectors(cfg PipelineConfig) (actionport.Effector, *watch.Service, error) {
	watch, err := watch.New(watch.Config{DB: cfg.DB, RuntimeOwner: runtimeOwnershipCheck(cfg), Epoch: cfg.OwnerEpoch, Interlock: interlock.DurableReader{}, Clock: cfg.Clock})
	if err != nil {
		return nil, nil, fmt.Errorf("compose watch: %w", err)
	}
	gatewayEffector := cfg.GatewayEffector
	if gatewayEffector != nil {
		gatewayEffector.WithTelemetry(cfg.Telemetry)
	}
	compositeEffector := app.NewCompositeEffector(watch, cfg.Effector)
	if gatewayEffector != nil {
		compositeEffector.WithSerial(gatewayEffector)
	}
	return compositeEffector, watch, nil
}

func composePipeline(cfg PipelineConfig, log *eventlog.EventLog, stream *engine.Engine, watch *watch.Service) (*app.Pipeline, error) {
	episodeService, err := composeEpisodes(cfg)
	if err != nil {
		return nil, err
	}
	policyGateway, dispatcher, err := composeGovernance(cfg)
	if err != nil {
		return nil, err
	}
	return app.NewPipeline(app.PipelineDependencies{
		Log: log, Engine: stream, Admission: composeAdmission(cfg, episodeService), Runner: episodeService, Dispatcher: dispatcher, Watch: watch, Telemetry: cfg.Telemetry,
		Transactions: &store.PipelineStore{DB: cfg.DB, Policy: policyGateway, Owner: cfg.Owner, OwnerEpoch: cfg.OwnerEpoch},
		Sources:      &transport.Sources{DB: cfg.DB, Log: log, TenantID: cfg.TenantID, Telemetry: cfg.Telemetry},
		Clock:        cfg.Clock, TenantID: cfg.TenantID,
	}), nil
}

// composeGovernance builds the policy plane and the action plane that executes
// what policy approves.
func composeGovernance(cfg PipelineConfig) (*policy.Service, *actions.Service, error) {
	policyGateway, err := composePolicy(cfg)
	if err != nil {
		return nil, nil, err
	}
	dispatcher, err := composeDispatcher(cfg)
	if err != nil {
		return nil, nil, err
	}
	return policyGateway, dispatcher, nil
}

func composeAdmission(cfg PipelineConfig, episodeService *episodes.Service) *admission.Admitter {
	return admission.New(admission.Config{DB: cfg.DB, Episodes: episodeService, Clock: cfg.Clock, TenantID: cfg.TenantID, Owner: cfg.Owner, OwnerEpoch: cfg.OwnerEpoch, EpochControl: cfg.EpochControl, DemoMode: cfg.DemoMode})
}

func composeEpisodes(cfg PipelineConfig) (*episodes.Service, error) {
	service, err := episodes.New(episodes.Config{Spec: cfg.Spec, IDGenerator: cfg.IDGenerator, CostControl: &costcontrol.Controller{}, Execution: &episodes.ExecutionConfig{DB: cfg.DB, Executor: cfg.Executor, Clock: cfg.Clock, OwnerEpoch: cfg.OwnerEpoch, DecisionEpoch: policyEpochCheck(cfg), ShadowStore: &qualification.ShadowStore{DB: cfg.DB}, Telemetry: cfg.Telemetry}})
	if err != nil {
		return nil, fmt.Errorf("compose episodes: %w", err)
	}
	return service, nil
}

func composePolicy(cfg PipelineConfig) (*policy.Service, error) {
	service, err := policy.New(policy.Config{PolicyVersion: cfg.Spec.Digest, IDGenerator: cfg.IDGenerator, OwnerEpoch: cfg.OwnerEpoch, RuntimeOwner: runtimeOwnershipCheck(cfg), DecisionEpoch: policyEpochCheck(cfg), Interlock: interlock.DurableReader{}, Calibration: policyCalibrationCheck(cfg)})
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

func runtimeOwnershipCheck(cfg PipelineConfig) func(context.Context, *sql.Tx, string) error {
	if cfg.Owner == nil || cfg.OwnerEpoch == "" {
		return unownedCheck
	}
	return cfg.Owner.Assert
}

func policyEpochCheck(cfg PipelineConfig) func(context.Context, *sql.Tx, string) error {
	if cfg.EpochControl == nil {
		return unownedCheck
	}
	return cfg.EpochControl.AssertDecisionTx
}

func unownedCheck(context.Context, *sql.Tx, string) error { return nil }

func composeDispatcher(cfg PipelineConfig) (*actions.Service, error) {
	service, err := actions.New(actions.Config{DB: cfg.DB, Effector: cfg.Effector, RuntimeOwner: runtimeOwnershipCheck(cfg), Epoch: cfg.OwnerEpoch,
		Interlock: interlock.DurableReader{}, Clock: cfg.Clock, IDs: cfg.IDGenerator, LeaseOwner: "runtime-actions/" + cfg.OwnerEpoch,
		LeaseFor: time.Minute, Telemetry: cfg.Telemetry})
	if err != nil {
		return nil, fmt.Errorf("compose actions: %w", err)
	}
	return service, nil
}

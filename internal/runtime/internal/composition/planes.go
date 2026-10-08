package composition

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	app "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

func pipelineDefaults(cfg PipelineConfig) PipelineConfig {
	if cfg.TenantID == "" {
		cfg.TenantID = contractsv1.TenantID
	}
	cfg.Clock = sources.OrPhysical(cfg.Clock)
	cfg.IDGenerator = sources.OrRandom(cfg.IDGenerator)
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
	watch, err := watch.New(watch.Config{DB: cfg.DB, RuntimeOwner: runtimeOwnershipCheck(cfg), Epoch: cfg.OwnerEpoch, Clock: cfg.Clock})
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

func composePipeline(cfg PipelineConfig, log *eventlog.EventLog, stream *engine.Service, watch *watch.Service) (*app.Pipeline, error) {
	episodeService, err := composeEpisodes(cfg)
	if err != nil {
		return nil, err
	}
	policyGateway, dispatcher, err := composeGovernance(cfg)
	if err != nil {
		return nil, err
	}
	transactions := &store.PipelineStore{DB: cfg.DB, Policy: policyGateway, RuntimeOwner: runtimeOwnershipCheck(cfg), OwnerEpoch: cfg.OwnerEpoch, Episodes: episodeService, TenantID: cfg.TenantID}
	admitter, err := composeAdmission(cfg, transactions)
	if err != nil {
		return nil, err
	}
	return app.NewPipeline(pipelineDependencies(cfg, log, stream, watch, admitter, episodeService, dispatcher, transactions)), nil
}

func pipelineDependencies(cfg PipelineConfig, log *eventlog.EventLog, stream *engine.Service, watch *watch.Service, admitter *app.Admitter, runner *episodes.Service, dispatcher *actions.Service, transactions *store.PipelineStore) app.PipelineDependencies {
	return app.PipelineDependencies{
		Log: log, Engine: stream, Admission: admitter, Runner: runner, Dispatcher: dispatcher, Watch: watch, Telemetry: cfg.Telemetry,
		Transactions: transactions,
		Sources:      &transport.Sources{DB: cfg.DB, Log: log, TenantID: cfg.TenantID, Telemetry: cfg.Telemetry},
		Clock:        cfg.Clock, TenantID: cfg.TenantID,
	}
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

func composeAdmission(cfg PipelineConfig, transactions *store.PipelineStore) (*app.Admitter, error) {
	admitter, err := app.NewAdmitter(app.AdmitterConfig{Store: transactions, Clock: cfg.Clock, OwnerEpoch: cfg.OwnerEpoch, EpochControl: cfg.EpochControl, DemoMode: cfg.DemoMode})
	if err != nil {
		return nil, fmt.Errorf("compose admission: %w", err)
	}
	return admitter, nil
}

func composeEpisodes(cfg PipelineConfig) (*episodes.Service, error) {
	service, err := episodes.New(episodes.Config{Spec: cfg.Spec, IDGenerator: cfg.IDGenerator, CostControl: &runtimecontrol.CostLedger{}, Execution: &episodes.ExecutionConfig{DB: cfg.DB, Executor: cfg.Executor, Clock: cfg.Clock, OwnerEpoch: cfg.OwnerEpoch, RuntimeOwner: runtimeOwnershipCheck(cfg), DecisionEpoch: policyEpochCheck(cfg), Telemetry: cfg.Telemetry}})
	if err != nil {
		return nil, fmt.Errorf("compose episodes: %w", err)
	}
	return service, nil
}

func composePolicy(cfg PipelineConfig) (*policy.Service, error) {
	service, err := policy.New(policy.Config{PolicyVersion: cfg.Spec.Digest, IDGenerator: cfg.IDGenerator, OwnerEpoch: cfg.OwnerEpoch, RuntimeOwner: runtimeOwnershipCheck(cfg), DecisionEpoch: policyEpochCheck(cfg)})
	if err != nil {
		return nil, fmt.Errorf("compose policy: %w", err)
	}
	return service, nil
}

func runtimeOwnershipCheck(cfg PipelineConfig) storage.OwnerCheck {
	if cfg.Owner == nil {
		return engine.ReplayOwnership
	}
	return cfg.Owner.Assert
}

func policyEpochCheck(cfg PipelineConfig) storage.OwnerCheck {
	if cfg.EpochControl == nil {
		return engine.ReplayOwnership
	}
	return cfg.EpochControl.AssertDecisionTx
}

func composeDispatcher(cfg PipelineConfig) (*actions.Service, error) {
	effector, ok := cfg.Effector.(actionport.AuthorizedEffector)
	if !ok {
		return nil, fmt.Errorf("compose actions: effector must enforce dispatch authorization")
	}
	service, err := actions.New(actions.Config{DB: cfg.DB, Effector: effector, RuntimeOwner: runtimeOwnershipCheck(cfg), Epoch: cfg.OwnerEpoch,
		Clock: cfg.Clock, IDs: cfg.IDGenerator, LeaseOwner: "runtime-actions/" + cfg.OwnerEpoch,
		Telemetry: cfg.Telemetry})
	if err != nil {
		return nil, fmt.Errorf("compose actions: %w", err)
	}
	return service, nil
}

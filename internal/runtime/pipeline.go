package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

const watchReadBatchSize = 1000

// PipelineConfig configures one owner-scoped live runtime pipeline.
type PipelineConfig struct {
	DB                *storage.DB
	Spec              *spec.CompiledSpec
	TenantID          string
	Owner             *storage.RuntimeOwner
	OwnerEpoch        string
	Clock             clock.Clock
	Executor          episodes.Executor
	Effector          actions.Effector
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
	EpochControl *storage.EpochControl
	// SerialEffector is optional and supplies the explicitly routed thermal
	// action boundary. It is never used by replay or shadow execution.
	SerialEffector *actions.SerialEffector
}

// PipelineReport describes one completed live batch.
type PipelineReport struct {
	EventsIngested     int
	EventsProcessed    int
	EpisodesAdmitted   int
	EpisodesExecuted   int
	IntentsEvaluated   int
	CommandsDispatched int
}

// Pipeline composes the deterministic stream, cognition, episode, policy,
// and action planes. It is intentionally batch-oriented at this stage: the
// same methods are called repeatedly by a future continuous ingestion loop.
type Pipeline struct {
	db           *storage.DB
	log          *eventlog.EventLog
	engine       *engine.Engine
	assembler    *episodes.Assembler
	runner       *episodes.Runner
	policy       *policy.Gateway
	dispatcher   *actions.Dispatcher
	watch        *actions.WatchEffector
	owner        *storage.RuntimeOwner
	ownerEpoch   string
	clk          clock.Clock
	tenantID     string
	watchMu      sync.Mutex
	watchStop    context.CancelFunc
	watchDone    chan struct{}
	watchErr     error
	telemetry    *telemetry.Runtime
	demoMode     bool
	epochControl *storage.EpochControl
}

// ErrFixtureRejected is returned when a production pipeline (no --demo-mode)
// admits a scheduler item whose executor is `fixture`.
var ErrFixtureRejected = errors.New("fixture executor rejected")

// NewPipeline creates a fully composed live pipeline. The caller must start
// the runtime Service first when Owner is configured.
func NewPipeline(ctx context.Context, cfg PipelineConfig) (*Pipeline, error) {
	if cfg.DB == nil || cfg.Spec == nil {
		return nil, fmt.Errorf("pipeline database and spec are required")
	}
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
	watch := actions.NewWatchEffectorWithClock(cfg.DB, cfg.Clock)
	watch.WithRuntimeOwner(cfg.Owner, cfg.OwnerEpoch)
	watch.WithInterlock(interlock.DurableReader{})
	serialEffector := cfg.SerialEffector
	if serialEffector != nil {
		serialEffector.WithTelemetry(cfg.Telemetry)
	}
	compositeEffector := actions.NewCompositeEffector(watch, cfg.Effector)
	compositeEffector.WithSerial(serialEffector)
	cfg.Effector = compositeEffector
	log := eventlog.NewEventLogWithClock(cfg.DB, cfg.Clock)
	stream, err := engine.NewEngine(ctx, cfg.DB, log, cfg.Clock, cfg.Spec, cfg.TenantID)
	if err != nil {
		return nil, fmt.Errorf("create stream engine: %w", err)
	}
	stream.WithRuntimeOwner(cfg.Owner, cfg.OwnerEpoch)
	if cfg.GlobalCostCeiling != nil || cfg.TenantCostCeiling != nil || cfg.CostKillSwitch != nil {
		if err := configureCostLimits(ctx, cfg); err != nil {
			return nil, err
		}
	}
	assembler := episodes.NewAssembler(cfg.Spec, cfg.IDGenerator)
	assembler.WithCostControl(&costcontrol.Controller{})
	runner := episodes.NewRunnerWithEpoch(cfg.DB, cfg.Executor, cfg.Clock, cfg.IDGenerator, cfg.OwnerEpoch)
	runner.WithAssembler(assembler)
	runner.WithCostControl(&costcontrol.Controller{})
	runner.WithEpochControl(cfg.EpochControl)
	runner.WithShadowStore(&storage.ShadowStore{DB: cfg.DB})
	runner.WithTelemetry(cfg.Telemetry)
	policyGateway := policy.NewGatewayWithOwner(cfg.Spec.Digest, cfg.IDGenerator, cfg.Owner, cfg.OwnerEpoch)
	policyGateway.WithInterlock(interlock.DurableReader{})
	policyGateway.WithCalibration(&storage.CalibrationStore{DB: cfg.DB})
	policyGateway.WithEpochControl(cfg.EpochControl)
	dispatcher := actions.NewDispatcher(cfg.DB, cfg.Effector, cfg.Clock, cfg.IDGenerator, "runtime-actions/"+cfg.OwnerEpoch, time.Minute)
	dispatcher.WithRuntimeOwner(cfg.Owner, cfg.OwnerEpoch)
	dispatcher.WithInterlock(interlock.DurableReader{})
	dispatcher.WithTelemetry(cfg.Telemetry)
	return &Pipeline{
		db:           cfg.DB,
		log:          log,
		engine:       stream,
		assembler:    assembler,
		runner:       runner,
		policy:       policyGateway,
		dispatcher:   dispatcher,
		watch:        watch,
		telemetry:    cfg.Telemetry,
		owner:        cfg.Owner,
		ownerEpoch:   cfg.OwnerEpoch,
		clk:          cfg.Clock,
		tenantID:     cfg.TenantID,
		demoMode:     cfg.DemoMode,
		epochControl: cfg.EpochControl,
	}, nil
}

// Start begins runtime-owned maintenance loops. It is safe to call once for
// a pipeline; the caller should call Close when the live runtime stops.
func (p *Pipeline) Start(ctx context.Context) error {
	if p == nil || p.watch == nil {
		return fmt.Errorf("pipeline watch maintenance is not configured")
	}
	p.watchMu.Lock()
	defer p.watchMu.Unlock()
	if p.watchStop != nil {
		return fmt.Errorf("pipeline is already started")
	}
	watchCtx, stop := context.WithCancel(ctx)
	p.watchStop = stop
	p.watchDone = make(chan struct{})
	done := p.watchDone
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				if err := p.watch.Expire(watchCtx); err != nil {
					p.watchMu.Lock()
					p.watchErr = err
					p.watchMu.Unlock()
					return
				}
			}
		}
	}()
	return nil
}

// Close stops runtime-owned maintenance loops.
func (p *Pipeline) Close() error {
	if p == nil {
		return nil
	}
	p.watchMu.Lock()
	stop, done := p.watchStop, p.watchDone
	p.watchStop = nil
	p.watchDone = nil
	p.watchMu.Unlock()
	if stop != nil {
		stop()
		<-done
	}
	return nil
}

func (p *Pipeline) watchFailure() error {
	p.watchMu.Lock()
	defer p.watchMu.Unlock()
	return p.watchErr
}

func (p *Pipeline) assertOwner(ctx context.Context) error {
	if p.owner == nil || p.ownerEpoch == "" {
		return nil
	}
	if err := p.db.WithTx(ctx, func(tx *sql.Tx) error {
		return p.owner.Assert(ctx, tx, p.ownerEpoch)
	}); err != nil {
		return fmt.Errorf("runtime ownership lost: %w", err)
	}
	return nil
}

func (p *Pipeline) assertOwnerTx(ctx context.Context, tx *sql.Tx) error {
	if p.owner == nil || p.ownerEpoch == "" {
		return nil
	}
	if err := p.owner.Assert(ctx, tx, p.ownerEpoch); err != nil {
		return fmt.Errorf("runtime ownership lost: %w", err)
	}
	return nil
}

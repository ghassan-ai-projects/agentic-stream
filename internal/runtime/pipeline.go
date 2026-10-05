package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/admission"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

const watchReadBatchSize = 1000

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
	db         *storage.DB
	log        *eventlog.EventLog
	engine     *engine.Engine
	admission  *admission.Admitter
	runner     *episodes.Runner
	policy     *policy.Gateway
	dispatcher *actions.Dispatcher
	watch      *watch.Effector
	owner      *runtimecontrol.RuntimeOwner
	ownerEpoch string
	clk        clock.Clock
	tenantID   string
	watchMu    sync.Mutex
	watchStop  context.CancelFunc
	watchDone  chan struct{}
	watchErr   error
	telemetry  *telemetry.Runtime
}

// NewPipeline creates a fully composed live pipeline. The caller must start
// the runtime Service first when Owner is configured.
func NewPipeline(ctx context.Context, cfg PipelineConfig) (*Pipeline, error) {
	if cfg.DB == nil || cfg.Spec == nil {
		return nil, fmt.Errorf("pipeline database and spec are required")
	}
	cfg = pipelineDefaults(cfg)
	var watch *watch.Effector
	cfg.Effector, watch = composeEffectors(cfg)
	log := eventlog.NewEventLogWithClock(cfg.DB, cfg.Clock)
	stream, err := newOwnedStream(ctx, cfg, log)
	if err != nil {
		return nil, err
	}
	if err := configureCostLimits(ctx, cfg); err != nil {
		return nil, err
	}
	return composePipeline(cfg, log, stream, watch), nil
}

func newOwnedStream(ctx context.Context, cfg PipelineConfig, log *eventlog.EventLog) (*engine.Engine, error) {
	stream, err := engine.NewEngine(ctx, cfg.DB, log, cfg.Clock, cfg.Spec, cfg.TenantID)
	if err != nil {
		return nil, fmt.Errorf("create stream engine: %w", err)
	}
	stream.WithRuntimeOwner(cfg.Owner, cfg.OwnerEpoch)
	return stream, nil
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
	go p.maintainWatches(watchCtx, done)
	return nil
}

func (p *Pipeline) maintainWatches(watchCtx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-watchCtx.Done():
			return
		case <-ticker.C:
			if err := p.expireMaintainedWatches(watchCtx); err != nil {
				return
			}
		}
	}
}

func (p *Pipeline) expireMaintainedWatches(ctx context.Context) error {
	if err := p.watch.Expire(ctx); err != nil {
		p.watchMu.Lock()
		p.watchErr = err
		p.watchMu.Unlock()
		return fmt.Errorf("%w", err)
	}
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

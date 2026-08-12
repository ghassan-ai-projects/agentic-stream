package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// PipelineConfig configures one owner-scoped live runtime pipeline.
type PipelineConfig struct {
	DB          *storage.DB
	Spec        *spec.CompiledSpec
	TenantID    string
	Owner       *storage.RuntimeOwner
	OwnerEpoch  string
	Clock       clock.Clock
	Executor    episodes.Executor
	Effector    actions.Effector
	IDGenerator ids.Generator
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
	assembler  *episodes.Assembler
	runner     *episodes.Runner
	policy     *policy.Gateway
	dispatcher *actions.Dispatcher
	owner      *storage.RuntimeOwner
	ownerEpoch string
	clk        clock.Clock
	tenantID   string
}

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
	log := eventlog.NewEventLogWithClock(cfg.DB, cfg.Clock)
	stream, err := engine.NewEngine(ctx, cfg.DB, log, cfg.Clock, cfg.Spec, cfg.TenantID)
	if err != nil {
		return nil, fmt.Errorf("create stream engine: %w", err)
	}
	stream.WithRuntimeOwner(cfg.Owner, cfg.OwnerEpoch)
	return &Pipeline{
		db:         cfg.DB,
		log:        log,
		engine:     stream,
		assembler:  episodes.NewAssembler(cfg.Spec, cfg.IDGenerator),
		runner:     episodes.NewRunnerWithEpoch(cfg.DB, cfg.Executor, cfg.Clock, cfg.IDGenerator, cfg.OwnerEpoch),
		policy:     policy.NewGatewayWithOwner(cfg.Spec.Digest, cfg.IDGenerator, cfg.Owner, cfg.OwnerEpoch),
		dispatcher: actions.NewDispatcher(cfg.DB, cfg.Effector, cfg.Clock, cfg.IDGenerator, "runtime-actions/"+cfg.OwnerEpoch, time.Minute).WithRuntimeOwner(cfg.Owner, cfg.OwnerEpoch),
		owner:      cfg.Owner,
		ownerEpoch: cfg.OwnerEpoch,
		clk:        cfg.Clock,
		tenantID:   cfg.TenantID,
	}, nil
}

// RunJSONL ingests normalized JSONL, evaluates the stream and cognition,
// executes all admitted episodes, applies policy, and dispatches approved
// commands. Each stage is idempotent against the durable ledgers.
func (p *Pipeline) RunJSONL(ctx context.Context, path string) (PipelineReport, error) {
	if p == nil {
		return PipelineReport{}, fmt.Errorf("pipeline is nil")
	}
	if err := p.assertOwner(ctx); err != nil {
		return PipelineReport{}, err
	}
	var report PipelineReport
	replay := ingress.NewJSONLReplay(p.db, p.log, p.tenantID, path, "live-jsonl:"+path)
	var err error
	report.EventsIngested, err = replay.Run(ctx)
	if err != nil {
		return report, fmt.Errorf("ingest live JSONL: %w", err)
	}
	return p.runAfterIngest(ctx, report)
}

// RunSimulatorJSONL ingests the strict streams-simulator adapter format and
// runs the same pipeline stages as RunJSONL.
func (p *Pipeline) RunSimulatorJSONL(ctx context.Context, path string) (PipelineReport, error) {
	if p == nil {
		return PipelineReport{}, fmt.Errorf("pipeline is nil")
	}
	if err := p.assertOwner(ctx); err != nil {
		return PipelineReport{}, err
	}
	replay := ingress.NewSimulatorJSONLReplay(p.db, p.log, ingress.SimulatorOptions{TenantID: p.tenantID}, path, "live-simulator:"+path)
	count, err := replay.Run(ctx)
	if err != nil {
		return PipelineReport{}, fmt.Errorf("ingest simulator JSONL: %w", err)
	}
	return p.runAfterIngest(ctx, PipelineReport{EventsIngested: count})
}

func (p *Pipeline) runAfterIngest(ctx context.Context, report PipelineReport) (PipelineReport, error) {
	processed, err := p.engine.RunGlobal(ctx, nil)
	if err != nil {
		return report, fmt.Errorf("run live stream engine: %w", err)
	}
	report.EventsProcessed = processed
	if err := p.assertOwner(ctx); err != nil {
		return report, err
	}
	report.EpisodesAdmitted, err = p.assemblePending(ctx)
	if err != nil {
		return report, err
	}
	for {
		processed, runErr := p.runner.RunOnce(ctx, p.tenantID)
		if runErr != nil {
			return report, fmt.Errorf("run episode: %w", runErr)
		}
		if !processed {
			break
		}
		report.EpisodesExecuted++
	}
	if err := p.evaluatePendingIntents(ctx, &report); err != nil {
		return report, err
	}
	for {
		dispatched, dispatchErr := p.dispatcher.DispatchOnce(ctx)
		if dispatchErr != nil {
			return report, fmt.Errorf("dispatch action: %w", dispatchErr)
		}
		if !dispatched {
			break
		}
		report.CommandsDispatched++
	}
	return report, nil
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

func (p *Pipeline) assemblePending(ctx context.Context) (int, error) {
	count := 0
	for {
		var itemID string
		now := p.clk.Now().UTC()
		err := p.db.QueryRowContext(ctx, `
			SELECT scheduler_item_id FROM scheduler_items
			WHERE tenant_id = ? AND status = 'pending' AND (not_before IS NULL OR not_before <= ?)
			ORDER BY not_before, created_at, scheduler_item_id LIMIT 1`,
			p.tenantID, now.Format(time.RFC3339Nano)).Scan(&itemID)
		if errors.Is(err, sql.ErrNoRows) {
			return count, nil
		}
		if err != nil {
			return count, fmt.Errorf("find pending scheduler item: %w", err)
		}
		if err := p.db.WithTx(ctx, func(tx *sql.Tx) error {
			if err := p.assertOwnerTx(ctx, tx); err != nil {
				return err
			}
			req, err := p.assembler.Assemble(ctx, tx, itemID, p.tenantID)
			if err != nil {
				return err
			}
			return p.assembler.Persist(ctx, tx, req, now)
		}); err != nil {
			return count, fmt.Errorf("admit scheduler item %s: %w", itemID, err)
		}
		count++
	}
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

func (p *Pipeline) evaluatePendingIntents(ctx context.Context, report *PipelineReport) error {
	for {
		var intentID string
		err := p.db.QueryRowContext(ctx, `
			SELECT intent_id FROM intents WHERE tenant_id = ? AND policy_status = 'pending'
			ORDER BY created_at, intent_id LIMIT 1`, p.tenantID).Scan(&intentID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find pending intent: %w", err)
		}
		now := p.clk.Now().UTC()
		if err := p.db.WithTx(ctx, func(tx *sql.Tx) error {
			_, err := p.policy.EvaluateIntent(ctx, tx, intentID, now)
			return err
		}); err != nil {
			return fmt.Errorf("evaluate intent %s: %w", intentID, err)
		}
		report.IntentsEvaluated++
	}
}

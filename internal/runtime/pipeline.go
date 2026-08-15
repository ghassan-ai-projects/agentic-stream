package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

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
	watch      *actions.WatchEffector
	owner      *storage.RuntimeOwner
	ownerEpoch string
	clk        clock.Clock
	tenantID   string
	watchMu    sync.Mutex
	watchStop  context.CancelFunc
	watchDone  chan struct{}
	watchErr   error
	telemetry  *telemetry.Runtime
	demoMode   bool
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
	watch := actions.NewWatchEffectorWithClock(cfg.DB, cfg.Clock).WithRuntimeOwner(cfg.Owner, cfg.OwnerEpoch).WithInterlock(interlock.DurableReader{})
	cfg.Effector = actions.NewCompositeEffector(watch, cfg.Effector)
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
	return &Pipeline{
		db:         cfg.DB,
		log:        log,
		engine:     stream,
		assembler:  episodes.NewAssembler(cfg.Spec, cfg.IDGenerator).WithCostControl(&costcontrol.Controller{}),
		runner:     episodes.NewRunnerWithEpoch(cfg.DB, cfg.Executor, cfg.Clock, cfg.IDGenerator, cfg.OwnerEpoch).WithCostControl(&costcontrol.Controller{}).WithEpochControl(cfg.EpochControl).WithShadowStore(&storage.ShadowStore{DB: cfg.DB}).WithTelemetry(cfg.Telemetry),
		policy:     policy.NewGatewayWithOwner(cfg.Spec.Digest, cfg.IDGenerator, cfg.Owner, cfg.OwnerEpoch).WithInterlock(interlock.DurableReader{}).WithCalibration(&storage.CalibrationStore{DB: cfg.DB}).WithEpochControl(cfg.EpochControl),
		dispatcher: actions.NewDispatcher(cfg.DB, cfg.Effector, cfg.Clock, cfg.IDGenerator, "runtime-actions/"+cfg.OwnerEpoch, time.Minute).WithRuntimeOwner(cfg.Owner, cfg.OwnerEpoch).WithInterlock(interlock.DurableReader{}),
		watch:      watch,
		telemetry:  cfg.Telemetry,
		owner:      cfg.Owner,
		ownerEpoch: cfg.OwnerEpoch,
		clk:        cfg.Clock,
		tenantID:   cfg.TenantID,
		demoMode:   cfg.DemoMode,
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

func configureCostLimits(ctx context.Context, cfg PipelineConfig) error {
	if err := cfg.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if cfg.Owner != nil && cfg.OwnerEpoch != "" {
			if err := cfg.Owner.Assert(ctx, tx, cfg.OwnerEpoch); err != nil {
				return fmt.Errorf("assert owner for cost configuration: %w", err)
			}
		}
		if cfg.GlobalCostCeiling != nil || cfg.CostKillSwitch != nil {
			maxMicro, existingKill, err := readCostLimit(ctx, tx, "global")
			if err != nil {
				return fmt.Errorf("read global cost limit: %w", err)
			}
			if cfg.GlobalCostCeiling != nil {
				maxMicro = *cfg.GlobalCostCeiling
			}
			kill := existingKill
			if cfg.CostKillSwitch != nil {
				kill = *cfg.CostKillSwitch
			}
			if err := costcontrol.SetLimit(ctx, tx, "global", "", maxMicro, kill, cfg.Clock.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				return fmt.Errorf("set global cost limit: %w", err)
			}
		}
		if cfg.TenantCostCeiling != nil {
			kill := false
			if cfg.CostKillSwitch != nil {
				kill = *cfg.CostKillSwitch
			}
			if err := costcontrol.SetLimit(ctx, tx, "tenant:"+cfg.TenantID, cfg.TenantID, *cfg.TenantCostCeiling, kill, cfg.Clock.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				return fmt.Errorf("set tenant cost limit: %w", err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("configure cost limits: %w", err)
	}
	return nil
}

func readCostLimit(ctx context.Context, tx *sql.Tx, scopeKey string) (uint64, bool, error) {
	var maxMicro int64
	var kill int
	if err := tx.QueryRowContext(ctx, "SELECT max_micro, kill_switch FROM cost_limits WHERE scope_key = ?", scopeKey).Scan(&maxMicro, &kill); err != nil {
		return 0, false, fmt.Errorf("load %s: %w", scopeKey, err)
	}
	if maxMicro < 0 {
		return 0, false, fmt.Errorf("cost limit %s is negative", scopeKey)
	}
	return uint64(maxMicro), kill != 0, nil
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
	before, err := p.currentEventPosition(ctx)
	if err != nil {
		return PipelineReport{}, err
	}
	var report PipelineReport
	replay := ingress.NewJSONLReplay(p.db, p.log, p.tenantID, path, "live-jsonl:"+path)
	report.EventsIngested, err = replay.Run(ctx)
	if err != nil {
		return report, fmt.Errorf("ingest live JSONL: %w", err)
	}
	return p.runAfterIngest(ctx, report, before)
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
	before, err := p.currentEventPosition(ctx)
	if err != nil {
		return PipelineReport{}, err
	}
	replay := ingress.NewSimulatorJSONLReplay(p.db, p.log, ingress.SimulatorOptions{TenantID: p.tenantID}, path, "live-simulator:"+path)
	count, err := replay.Run(ctx)
	if err != nil {
		return PipelineReport{}, fmt.Errorf("ingest simulator JSONL: %w", err)
	}
	return p.runAfterIngest(ctx, PipelineReport{EventsIngested: count}, before)
}

func (p *Pipeline) runAfterIngest(ctx context.Context, report PipelineReport, before eventlog.LogPosition) (result PipelineReport, err error) {
	ctx, span := telemetry.StartSpan(ctx, "agentic_stream.pipeline.batch", trace.WithSpanKind(trace.SpanKindConsumer))
	defer func() {
		if err != nil {
			telemetry.RecordError(span, err)
		}
		span.SetAttributes(
			attribute.Int("agentic_stream.events_ingested", report.EventsIngested),
			attribute.Int("agentic_stream.events_processed", report.EventsProcessed),
			attribute.Int("agentic_stream.episodes_admitted", report.EpisodesAdmitted),
			attribute.Int("agentic_stream.episodes_executed", report.EpisodesExecuted),
			attribute.Int("agentic_stream.intents_evaluated", report.IntentsEvaluated),
			attribute.Int("agentic_stream.commands_dispatched", report.CommandsDispatched),
		)
		span.End()
	}()
	if err := p.watchFailure(); err != nil {
		return report, fmt.Errorf("watch maintenance failed: %w", err)
	}
	if err := p.watch.Expire(ctx); err != nil {
		return report, fmt.Errorf("expire watch conditions: %w", err)
	}
	processed, err := p.engine.RunGlobal(ctx, nil)
	if err != nil {
		return report, fmt.Errorf("run live stream engine: %w", err)
	}
	report.EventsProcessed = processed
	if err := p.fireRecentWatches(ctx, before, span); err != nil {
		return report, fmt.Errorf("fire watches: %w", err)
	}
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
	if p.telemetry != nil {
		p.telemetry.ObservePipeline(telemetry.PipelineReport{
			EventsIngested: report.EventsIngested, EventsProcessed: report.EventsProcessed,
			EpisodesAdmitted: report.EpisodesAdmitted, EpisodesExecuted: report.EpisodesExecuted,
			IntentsEvaluated: report.IntentsEvaluated, CommandsDispatched: report.CommandsDispatched,
		})
	}
	return report, nil
}

func (p *Pipeline) currentEventPosition(ctx context.Context) (eventlog.LogPosition, error) {
	var position int64
	if err := p.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), 0) FROM event_log WHERE tenant_id = ?", p.tenantID).Scan(&position); err != nil {
		return 0, fmt.Errorf("read event position: %w", err)
	}
	return eventlog.LogPosition(position), nil
}

func (p *Pipeline) fireRecentWatches(ctx context.Context, before eventlog.LogPosition, span trace.Span) error {
	if err := p.log.Read(ctx, eventlog.ReadRequest{TenantID: p.tenantID, PartitionID: -1, AfterPosition: before, Limit: 100000}, func(record eventlog.Record) error {
		telemetry.AddLinkFromW3C(span, record.Envelope.Traceparent, record.Envelope.Tracestate)
		if _, err := p.watch.FireEvent(ctx, record.EventID, record.EntityID, record.Envelope.Data); err != nil {
			return fmt.Errorf("event %s: %w", record.EventID, err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("read recent events: %w", err)
	}
	return nil
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
		// P8 (drain): while the epoch is draining or killed, no NEW episode is
		// admitted — in-flight episodes finish under their recorded epoch.
		// This is a SKIP, not a batch error: the loop keeps running so the
		// runner drains the admitted backlog.
		if p.epochControl != nil {
			if err := p.epochControl.AssertAdmission(ctx, p.ownerEpoch); err != nil {
				return count, nil
			}
		}
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
		var requestKind, situationID string
		if err := p.db.WithTx(ctx, func(tx *sql.Tx) error {
			if err := p.assertOwnerTx(ctx, tx); err != nil {
				return fmt.Errorf("assert pipeline owner: %w", err)
			}
			req, err := p.assembler.Assemble(ctx, tx, itemID, p.tenantID)
			if err != nil {
				return fmt.Errorf("assemble scheduler item: %w", err)
			}
			// P8 (mode matrix): a production pipeline rejects the `fixture`
			// executor — it exists for demos and tests only, never on a live
			// route. The policy epoch is stamped ONCE here, never rewritten.
			if !p.demoMode && req.ExecutorName == "fixture" {
				return fmt.Errorf("%w: fixture executor %s on a production route",
					ErrFixtureRejected, req.ExecutorName)
			}
			req.PolicyEpoch = p.ownerEpoch
			requestKind = req.Kind
			situationID = req.SituationID
			return p.assembler.Persist(ctx, tx, req, now)
		}); err != nil {
			if errors.Is(err, costcontrol.ErrReservationRejected) {
				if skipErr := p.skipCostRejectedSchedulerItem(ctx, itemID, now, err); skipErr != nil {
					return count, fmt.Errorf("record cost-rejected scheduler item %s: %w", itemID, skipErr)
				}
				slog.WarnContext(ctx, "episode admission skipped by cost control",
					"scheduler_item_id", itemID,
					"reason", err,
				)
				continue
			}
			if requestKind == "reconsider" && errors.Is(err, episodes.ErrLiveEpisodeConflict) {
				if skipErr := p.coalesceSkippedSchedulerItem(ctx, itemID, now); skipErr != nil {
					return count, fmt.Errorf("record skipped reconsideration %s: %w", itemID, skipErr)
				}
				slog.WarnContext(ctx, "reconsideration episode admission skipped",
					"scheduler_item_id", itemID,
					"situation_id", situationID,
					"reason", "one_live_episode_per_situation",
					"error", err,
				)
				continue
			}
			if errors.Is(err, ErrFixtureRejected) {
				// Quarantine the misconfigured item (loudly) instead of leaving
				// it pending forever, which would block the whole queue.
				if skipErr := p.coalesceSkippedSchedulerItem(ctx, itemID, now); skipErr != nil {
					return count, fmt.Errorf("record fixture-rejected scheduler item %s: %w", itemID, skipErr)
				}
				slog.ErrorContext(ctx, "episode admission refused: fixture executor on a production route",
					"scheduler_item_id", itemID,
					"situation_id", situationID,
					"error", err,
				)
				continue
			}
			return count, fmt.Errorf("admit scheduler item %s: %w", itemID, err)
		}
		count++
	}
}

func (p *Pipeline) skipCostRejectedSchedulerItem(ctx context.Context, schedulerItemID string, now time.Time, rejection error) error {
	return p.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := p.assertOwnerTx(ctx, tx); err != nil {
			return fmt.Errorf("assert pipeline owner: %w", err)
		}

		var triggerID string
		var reasonsJSON []byte
		if err := tx.QueryRowContext(ctx, `
			SELECT trigger_id, reasons_json FROM trigger_evaluations
			WHERE trigger_id = (SELECT trigger_id FROM scheduler_items WHERE scheduler_item_id = ?)`, schedulerItemID).
			Scan(&triggerID, &reasonsJSON); err != nil {
			return fmt.Errorf("load cost-rejected trigger evaluation: %w", err)
		}
		var reasons []string
		if len(reasonsJSON) > 0 {
			if err := json.Unmarshal(reasonsJSON, &reasons); err != nil {
				return fmt.Errorf("decode trigger evaluation reasons: %w", err)
			}
		}
		reasons = append(reasons, "episode admission rejected by cost control: "+rejection.Error())
		reasonsJSON, err := json.Marshal(reasons)
		if err != nil {
			return fmt.Errorf("encode trigger evaluation reasons: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE trigger_evaluations SET reasons_json = ? WHERE trigger_id = ?",
			reasonsJSON, triggerID); err != nil {
			return fmt.Errorf("record cost rejection reason: %w", err)
		}

		result, err := tx.ExecContext(ctx, `
			UPDATE scheduler_items SET status = 'coalesced', updated_at = ?
			WHERE scheduler_item_id = ? AND status = 'pending'`,
			now.UTC().Format(time.RFC3339Nano), schedulerItemID)
		if err != nil {
			return fmt.Errorf("skip cost-rejected scheduler item: %w", err)
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("cost-rejected scheduler item rows affected: %w", err)
		}
		if updated != 1 {
			return fmt.Errorf("scheduler item %s is no longer pending", schedulerItemID)
		}
		return nil
	})
}

func (p *Pipeline) coalesceSkippedSchedulerItem(ctx context.Context, schedulerItemID string, now time.Time) error {
	return p.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := p.assertOwnerTx(ctx, tx); err != nil {
			return fmt.Errorf("assert pipeline owner: %w", err)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE scheduler_items SET status = 'coalesced', updated_at = ?
			WHERE scheduler_item_id = ? AND status = 'pending'`,
			now.UTC().Format(time.RFC3339Nano), schedulerItemID,
		)
		if err != nil {
			return fmt.Errorf("coalesce scheduler item: %w", err)
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("coalesced scheduler item rows affected: %w", err)
		}
		if updated != 1 {
			return fmt.Errorf("scheduler item %s is no longer pending", schedulerItemID)
		}
		return nil
	})
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
			if err != nil {
				return fmt.Errorf("evaluate intent: %w", err)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("evaluate intent %s: %w", intentID, err)
		}
		report.IntentsEvaluated++
	}
}

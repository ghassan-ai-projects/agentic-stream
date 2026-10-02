package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

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

// RunLiveSocket serves normalized JSONL from a live Unix socket and advances
// the same event, situation, cognition, policy, and action pipeline used by a
// continuous file source. The live source is not replay: an emulator or
// physical effect profile may be selected by the caller's startup guards.
func (p *Pipeline) RunLiveSocket(ctx context.Context, path string) error {
	if p == nil {
		return fmt.Errorf("pipeline is nil")
	}
	if err := p.assertOwner(ctx); err != nil {
		return err
	}
	source := ingress.NewLiveUDSSource(p.log, p.tenantID, path).WithTelemetry(p.telemetry)
	err := source.Run(ctx, func(sinkCtx context.Context, env contractsv1.Envelope) error {
		if err := p.assertOwner(sinkCtx); err != nil {
			return err
		}
		before, err := p.currentEventPosition(sinkCtx)
		if err != nil {
			return err
		}
		positions, err := p.log.Append(sinkCtx, p.tenantID, []contractsv1.Envelope{env})
		if err != nil {
			return fmt.Errorf("append live event: %w", err)
		}
		if len(positions) != 1 || positions[0] < 0 {
			return nil
		}
		_, err = p.runAfterIngest(sinkCtx, PipelineReport{EventsIngested: 1}, before)
		return err
	})
	if err != nil && !normalLiveSocketShutdown(ctx, err) {
		return fmt.Errorf("run live socket source: %w", err)
	}
	return nil
}

func normalLiveSocketShutdown(ctx context.Context, err error) bool {
	return ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
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
	position, err := p.log.CurrentPosition(ctx, p.tenantID)
	if err != nil {
		return 0, fmt.Errorf("read event position: %w", err)
	}
	return position, nil
}

func (p *Pipeline) fireRecentWatches(ctx context.Context, before eventlog.LogPosition, span trace.Span) error {
	cursor := before
	for {
		read := 0
		lastPosition := cursor
		if err := p.log.Read(ctx, eventlog.ReadRequest{
			TenantID:      p.tenantID,
			PartitionID:   -1,
			AfterPosition: cursor,
			Limit:         watchReadBatchSize,
		}, func(record eventlog.Record) error {
			if record.Position <= cursor {
				return fmt.Errorf("event log did not advance past position %d", cursor)
			}
			read++
			lastPosition = record.Position
			telemetry.AddLinkFromW3C(span, record.Envelope.Traceparent, record.Envelope.Tracestate)
			if _, err := p.watch.FireEvent(ctx, record.EventID, record.EntityID, record.Envelope.Data); err != nil {
				return fmt.Errorf("event %s: %w", record.EventID, err)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("read recent events after position %d: %w", cursor, err)
		}
		if read < watchReadBatchSize {
			return nil
		}
		if lastPosition <= cursor {
			return fmt.Errorf("event log page did not advance past position %d", cursor)
		}
		cursor = lastPosition
	}
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

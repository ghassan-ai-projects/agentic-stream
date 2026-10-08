package app

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

func (p *Pipeline) RunJSONL(ctx context.Context, path string) (PipelineReport, error) {
	unlock, err := p.lockBatch()
	if err != nil {
		return PipelineReport{}, err
	}
	defer unlock()
	before, err := p.prepareIngest(ctx)
	if err != nil {
		return PipelineReport{}, err
	}
	var report PipelineReport
	report.EventsIngested, err = p.sources.RunJSONL(ctx, path)
	if err != nil {
		return report, fmt.Errorf("ingest live JSONL: %w", err)
	}
	return p.runAfterIngest(ctx, report, before)
}

func (p *Pipeline) prepareIngest(ctx context.Context) (eventlog.LogPosition, error) {
	if p == nil {
		return 0, fmt.Errorf("pipeline is nil")
	}
	if err := p.assertOwner(ctx); err != nil {
		return 0, err
	}
	return p.currentEventPosition(ctx)
}

func (p *Pipeline) RunLiveSocket(ctx context.Context, path string) error {
	if p == nil {
		return fmt.Errorf("pipeline is nil")
	}
	if err := p.assertOwner(ctx); err != nil {
		return err
	}
	err := p.sources.RunLiveSocket(ctx, path, func(sinkCtx context.Context, env contractsv1.Envelope) error {
		return p.ingestLiveEvent(sinkCtx, env)
	})
	if err != nil && !domain.NormalLiveSocketShutdown(ctx.Err(), err) {
		return fmt.Errorf("run live socket source: %w", err)
	}
	return nil
}

func (p *Pipeline) ingestLiveEvent(ctx context.Context, env contractsv1.Envelope) error {
	unlock, err := p.lockBatch()
	if err != nil {
		return err
	}
	defer unlock()
	before, err := p.prepareIngest(ctx)
	if err != nil {
		return err
	}
	appended, err := p.appendLiveEvent(ctx, env)
	if err != nil || !appended {
		return err
	}
	_, err = p.runAfterIngest(ctx, PipelineReport{EventsIngested: 1}, before)
	return err
}

func (p *Pipeline) appendLiveEvent(ctx context.Context, env contractsv1.Envelope) (bool, error) {
	positions, err := p.log.Append(ctx, p.tenantID, []contractsv1.Envelope{env})
	if err != nil {
		return false, fmt.Errorf("append live event: %w", err)
	}
	return len(positions) == 1 && positions[0] >= 0, nil
}

func (p *Pipeline) RunSimulatorJSONL(ctx context.Context, path string) (PipelineReport, error) {
	unlock, err := p.lockBatch()
	if err != nil {
		return PipelineReport{}, err
	}
	defer unlock()
	before, err := p.prepareIngest(ctx)
	if err != nil {
		return PipelineReport{}, err
	}
	count, err := p.sources.RunSimulatorJSONL(ctx, path)
	if err != nil {
		return PipelineReport{}, fmt.Errorf("ingest simulator JSONL: %w", err)
	}
	return p.runAfterIngest(ctx, PipelineReport{EventsIngested: count}, before)
}

func (p *Pipeline) runAfterIngest(ctx context.Context, report PipelineReport, before eventlog.LogPosition) (result PipelineReport, err error) {
	ctx, span := telemetry.StartSpan(ctx, "agentic_stream.pipeline.batch", trace.WithSpanKind(trace.SpanKindConsumer))
	defer func() { finishBatchSpan(span, report, err) }()
	err = p.advanceBatch(ctx, &report, before, span)
	return report, err
}

func finishBatchSpan(span trace.Span, report PipelineReport, err error) {
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
}

func (p *Pipeline) advanceBatch(ctx context.Context, report *PipelineReport, before eventlog.LogPosition, span trace.Span) error {
	if err := p.watchFailure(); err != nil {
		return fmt.Errorf("watch maintenance failed: %w", err)
	}
	if err := p.watch.Expire(ctx); err != nil {
		return fmt.Errorf("expire watch conditions: %w", err)
	}
	processed, err := p.engine.RunGlobal(ctx, nil)
	if err != nil {
		return fmt.Errorf("run live stream engine: %w", err)
	}
	report.EventsProcessed = processed
	if err := p.fireRecentWatches(ctx, before, span); err != nil {
		return fmt.Errorf("fire watches: %w", err)
	}
	return p.runGovernedBatch(ctx, report)
}

func (p *Pipeline) runGovernedBatch(ctx context.Context, report *PipelineReport) error {
	if err := p.assertOwner(ctx); err != nil {
		return err
	}
	var err error
	report.EpisodesAdmitted, err = p.admission.AdmitPending(ctx)
	if err != nil {
		return err //nolint:wrapcheck // Admission names the failed item; the batch error text is unchanged.
	}
	return p.runAdmittedBatch(ctx, report)
}

func (p *Pipeline) runAdmittedBatch(ctx context.Context, report *PipelineReport) error {
	if err := p.executeInlineEpisodes(ctx, report); err != nil {
		return err
	}
	if err := p.evaluatePendingIntents(ctx, report); err != nil {
		return err
	}
	if err := p.dispatchApprovedCommands(ctx, report); err != nil {
		return err
	}
	p.observeBatch(*report)
	return nil
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
		read, lastPosition, err := p.fireWatchPage(ctx, cursor, span)
		if err != nil {
			return err
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

func (p *Pipeline) fireWatchPage(ctx context.Context, cursor eventlog.LogPosition, span trace.Span) (int, eventlog.LogPosition, error) {
	read := 0
	lastPosition := cursor
	if err := p.log.Read(ctx, eventlog.ReadRequest{
		TenantID:      p.tenantID,
		PartitionID:   -1,
		AfterPosition: cursor,
		Limit:         watchReadBatchSize,
	}, func(record eventlog.Record) error {
		return p.fireWatchRecord(ctx, cursor, record, span, &read, &lastPosition)
	}); err != nil {
		return 0, 0, fmt.Errorf("read recent events after position %d: %w", cursor, err)
	}
	return read, lastPosition, nil
}

func (p *Pipeline) fireWatchRecord(ctx context.Context, cursor eventlog.LogPosition, record eventlog.Record, span trace.Span, read *int, lastPosition *eventlog.LogPosition) error {
	if record.Position <= cursor {
		return fmt.Errorf("event log did not advance past position %d", cursor)
	}
	(*read)++
	*lastPosition = record.Position
	telemetry.AddLinkFromW3C(span, record.Envelope.Traceparent, record.Envelope.Tracestate)
	if _, err := p.watch.FireEvent(ctx, record.EventID, record.EntityID, record.Envelope.Data); err != nil {
		return fmt.Errorf("event %s: %w", record.EventID, err)
	}
	return nil
}

func (p *Pipeline) evaluatePendingIntents(ctx context.Context, report *PipelineReport) error {
	for {
		intentID, found, err := p.transactions.NextPendingIntent(ctx, p.tenantID)
		if err != nil || !found {
			return err //nolint:wrapcheck // Policy names the failed read; the batch error text is unchanged.
		}
		if err := p.evaluateIntent(ctx, intentID); err != nil {
			return err
		}
		report.IntentsEvaluated++
	}
}

func (p *Pipeline) evaluateIntent(ctx context.Context, intentID string) error {
	return p.transactions.EvaluateIntent(ctx, intentID, p.clk.Now().UTC())
}

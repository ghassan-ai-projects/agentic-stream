package app

import (
	"context"
	"fmt"
	"slices"

	"go.opentelemetry.io/otel/trace"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

func (p *Pipeline) wakeWhatChanged(ctx context.Context, before eventlog.LogPosition, span trace.Span) error {
	if err := p.fireRecentWatches(ctx, before, span); err != nil {
		return fmt.Errorf("fire watches: %w", err)
	}
	unanswered, err := p.transactions.UnansweredReasoning(ctx)
	if err != nil {
		return fmt.Errorf("find failed reasoning: %w", err)
	}
	if err := p.retryReasoning(ctx, failedReasoning(unanswered)); err != nil {
		return fmt.Errorf("retry failed reasoning: %w", err)
	}
	return nil
}

func (p *Pipeline) fireRecentWatches(ctx context.Context, before eventlog.LogPosition, span trace.Span) error {
	cursor := before
	for {
		read, lastPosition, err := p.fireWatchPage(ctx, cursor, span)
		if err != nil {
			return err
		}
		if read < p.watchPage() {
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
		Limit:         p.watchPage(),
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
	fired, err := p.watch.FireEvent(ctx, record.EventID, record.EntityID, p.situationFeatures)
	if err != nil {
		return fmt.Errorf("event %s: %w", record.EventID, err)
	}
	if err := p.wakeSituations(ctx, slices.Compact(fired)); err != nil {
		return fmt.Errorf("wake the situations of event %s's watch fires: %w", record.EventID, err)
	}
	return nil
}

func (p *Pipeline) watchPage() int {
	if p.watchPageSize > 0 {
		return p.watchPageSize
	}
	return defaultWatchPageSize
}

func failedReasoning(unanswered []episodeledger.Unanswered) []engine.FailedReasoning {
	failed := make([]engine.FailedReasoning, len(unanswered))
	for i, episode := range unanswered {
		failed[i] = engine.FailedReasoning{SituationID: episode.SituationID, SituationVersion: episode.SituationVersion}
	}
	return failed
}

package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/transport"
)

// traceEpoch derives the virtual clock start: the earliest processing time
// among trace lines that pass the full ingress validity boundary.
func (s *replaySession) traceEpoch(ctx context.Context, tracePath string, log *eventlog.EventLog) (time.Time, error) {
	var earliest time.Time
	err := transport.TraceLines(tracePath, func(line []byte) {
		processingTime, valid := s.traceProcessingTime(ctx, line, log)
		if !valid {
			return
		}
		if earliest.IsZero() || processingTime.Before(earliest) {
			earliest = processingTime.UTC()
		}
	})
	if err != nil {
		return time.Time{}, err
	}
	return domain.EpochFromEarliest(earliest), nil
}

// traceProcessingTime shares ingress validity before a line can affect the clock.
func (s *replaySession) traceProcessingTime(ctx context.Context, line []byte, log *eventlog.EventLog) (time.Time, bool) {
	envelope, ok := domain.TraceEnvelope(line)
	if !ok {
		// JSONLReplay owns malformed-line quarantine. Epoch derivation is
		// only a clock bootstrap and must not turn a quarantinable line into
		// a whole-replay failure.
		return time.Time{}, false
	}
	if !s.validTraceEnvelope(ctx, &envelope, log) {
		return time.Time{}, false
	}
	return domain.EnvelopeProcessingTime(envelope)
}

func (s *replaySession) validTraceEnvelope(ctx context.Context, envelope *contractsv1.Envelope, log *eventlog.EventLog) bool {
	*envelope = domain.AdoptTenant(*envelope, s.tenantID)
	if !domain.ContractValidEnvelope(*envelope, s.tenantID) {
		return false
	}
	if err := log.ValidateEnvelope(ctx, *envelope); err != nil {
		return false
	}
	return true
}

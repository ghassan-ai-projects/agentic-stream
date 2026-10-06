package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// ServeLive listens on the Unix socket at path and hands each admitted envelope
// to sink, in arrival order, until ctx is canceled or the sink returns an
// error. It accepts reconnecting clients, applies bounded backpressure,
// quarantines malformed input, and never treats a client disconnect as a runtime
// failure. A canceled context is a normal shutdown and returns nil.
func (s *Service) ServeLive(ctx context.Context, path string, sink EnvelopeSink) error {
	if sink == nil {
		return errors.New("live UDS source sink is required")
	}
	if err := domain.ValidateSocketPath(path); err != nil {
		return err //nolint:wrapcheck // The domain rule names the unsafe path.
	}
	instanceID := sources.Random().New("live_uds_")
	cfg := transport.ServerConfig{Path: path, QueueSize: s.queueSize, Logger: s.logger}
	return transport.Serve(ctx, cfg, func(ctx context.Context, line domain.LiveLine) error { //nolint:wrapcheck // The transport wraps handler failures.
		return s.processLine(ctx, instanceID, line, sink)
	})
}

func (s *Service) processLine(ctx context.Context, instanceID string, item domain.LiveLine, sink EnvelopeSink) error {
	if item.ReadErr != nil {
		return s.rejectRaw(ctx, instanceID, item, domain.ReasonLineTooLarge, item.ReadErr)
	}
	verdict := s.admit(ctx, item.Data)
	if verdict.Decoded {
		return s.rejectEnvelope(ctx, item, verdict.Envelope, verdict.Reason, verdict.Cause)
	}
	if verdict.Rejected() {
		return s.rejectRaw(ctx, instanceID, item, verdict.Reason, verdict.Cause)
	}
	if s.telemetry != nil {
		s.telemetry.ObserveLiveLineIngested()
	}
	return sink(ctx, verdict.Envelope)
}

func (s *Service) rejectRaw(ctx context.Context, instanceID string, item domain.LiveLine, reason string, cause error) error {
	s.observeRejection(ctx, item, reason, cause)
	eventID := domain.LiveQuarantineID(instanceID, item.ConnectionID, item.LineNumber)
	if err := s.quarantineRaw(ctx, eventID, item.Data, reason, s.nowText()); err != nil {
		return fmt.Errorf("quarantine live line: %w", err)
	}
	return nil
}

func (s *Service) rejectEnvelope(ctx context.Context, item domain.LiveLine, env contractsv1.Envelope, reason string, cause error) error {
	s.observeRejection(ctx, item, reason, cause)
	if err := s.quarantineEnvelope(ctx, env, reason, s.nowText()); err != nil {
		return fmt.Errorf("quarantine live envelope: %w", err)
	}
	return nil
}

func (s *Service) observeRejection(ctx context.Context, item domain.LiveLine, reason string, cause error) {
	if s.telemetry != nil {
		s.telemetry.ObserveLiveLineRejected()
	}
	s.logger.WarnContext(ctx, "live ingress line rejected", "source", domain.LiveSourceTag, "connection_id", item.ConnectionID, "line_number", item.LineNumber, "reason_code", reason, "error", cause)
}

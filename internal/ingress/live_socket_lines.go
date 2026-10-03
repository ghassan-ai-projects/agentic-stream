package ingress

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func readLiveLine(reader *bufio.Reader) ([]byte, error) {
	line := make([]byte, 0, min(maxLiveSocketLineBytes, reader.Size()))
	for {
		part, err := reader.ReadSlice('\n')
		if len(line)+len(part) > maxLiveSocketLineBytes {
			// Preserve the bounded prefix for quarantine. The complete malformed
			// frame is intentionally not retained or allowed to grow memory.
			return line, errLiveSocketLineTooLarge
		}
		line = append(line, part...)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return completeLiveLine(line, err)
		}
	}
}

// completeLiveLine accepts a line ended by a newline or by end of input.
func completeLiveLine(line []byte, err error) ([]byte, error) {
	if err == nil || (errors.Is(err, io.EOF) && len(line) > 0) {
		return line, nil
	}
	return nil, fmt.Errorf("read live ingress line: %w", err)
}

func (s *LiveUDSSource) processLine(ctx context.Context, item liveLine, sink EnvelopeSink) error {
	if item.readErr != nil {
		return s.rejectRaw(ctx, item, "line_too_large", item.readErr)
	}
	verdict := admitEnvelopeLine(ctx, s.log, s.tenantID, item.data)
	if verdict.decoded {
		return s.rejectEnvelope(ctx, item, verdict.env, verdict.reason, verdict.cause)
	}
	if verdict.reason != "" {
		return s.rejectRaw(ctx, item, verdict.reason, verdict.cause)
	}
	if s.telemetry != nil {
		s.telemetry.ObserveLiveLineIngested()
	}
	return sink(ctx, verdict.env)
}

func (s *LiveUDSSource) rejectRaw(ctx context.Context, item liveLine, reason string, cause error) error {
	if s.telemetry != nil {
		s.telemetry.ObserveLiveLineRejected()
	}
	s.logger.WarnContext(ctx, "live ingress line rejected", "source", liveSocketSourceTag, "connection_id", item.connectionID, "line_number", item.lineNumber, "reason_code", reason, "error", cause)
	eventID := fmt.Sprintf("live-uds:%s:%d:%d", s.instanceID, item.connectionID, item.lineNumber)
	if err := s.log.QuarantineRaw(ctx, s.tenantID, eventID, item.data, reason, s.clk.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("quarantine live line: %w", err)
	}
	return nil
}

func (s *LiveUDSSource) rejectEnvelope(ctx context.Context, item liveLine, env contractsv1.Envelope, reason string, cause error) error {
	if s.telemetry != nil {
		s.telemetry.ObserveLiveLineRejected()
	}
	s.logger.WarnContext(ctx, "live ingress line rejected", "source", liveSocketSourceTag, "connection_id", item.connectionID, "line_number", item.lineNumber, "reason_code", reason, "error", cause)
	if err := s.log.QuarantineEnvelope(ctx, s.tenantID, env, reason, s.clk.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("quarantine live envelope: %w", err)
	}
	return nil
}

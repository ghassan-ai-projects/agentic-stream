package ingress

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"io"
	"time"
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
		if err == nil {
			return line, nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) && len(line) > 0 {
			return line, nil
		}
		return nil, fmt.Errorf("read live ingress line: %w", err)
	}
}

func (s *LiveUDSSource) processLine(ctx context.Context, item liveLine, sink EnvelopeSink) error {
	if item.readErr != nil {
		return s.rejectRaw(ctx, item, "line_too_large", item.readErr)
	}
	var env contractsv1.Envelope
	if err := json.Unmarshal(item.data, &env); err != nil {
		return s.rejectRaw(ctx, item, "malformed_json", err)
	}
	if env.TenantID == "" {
		env.TenantID = s.tenantID
	}
	if err := contractsv1.ValidateEnvelope(env, s.tenantID); err != nil {
		return s.rejectEnvelope(ctx, item, env, "envelope_invalid", err)
	}
	if err := s.log.ValidateEnvelope(ctx, env); err != nil {
		return s.rejectEnvelope(ctx, item, env, "schema_invalid", err)
	}
	if s.telemetry != nil {
		s.telemetry.ObserveLiveLineIngested()
	}
	return sink(ctx, env)
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

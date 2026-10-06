package app

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/transport"
)

const simulatorConnectorKind = "simulator-jsonl"

// ReplaySimulator appends all new event records from a streams-simulator trace
// from the connector's last checkpoint. It requires the trace's framing records
// and does not treat headers or trailers as events.
func (s *Service) ReplaySimulator(ctx context.Context, options domain.SimulatorOptions, path, connectorID string) (int, error) {
	connectorID = domain.SimulatorConnectorID(connectorID, path)
	f, err := transport.OpenTrace(path, "simulator trace")
	if err != nil {
		return 0, err //nolint:wrapcheck // The transport names the failed open.
	}
	defer func() { _ = f.Close() }()
	start, err := s.store.LoadLine(ctx, connectorID)
	if err != nil {
		return 0, err //nolint:wrapcheck // The store names the failed read.
	}
	trace := domain.NewSimulatorTrace(options)
	batch := envelopeBatch{log: s.log, tenantID: trace.Options().TenantID}
	err = s.replaySimulatorTrace(ctx, f, trace, start, connectorID, &batch)
	return batch.appended, err
}

// replaySimulatorTrace reads the whole trace, requires its framing records, then
// appends the final batch and checkpoints the last line.
func (s *Service) replaySimulatorTrace(ctx context.Context, f io.Reader, trace *domain.SimulatorTrace, start int, connectorID string, batch *envelopeBatch) error {
	if err := s.readSimulatorLines(ctx, f, trace, start, batch); err != nil {
		return err
	}
	if err := trace.Finish(); err != nil {
		return err //nolint:wrapcheck // The domain grammar names the missing framing.
	}
	if err := batch.flush(ctx); err != nil {
		return fmt.Errorf("append simulator batch: %w", err)
	}
	return s.store.SaveLine(ctx, connectorID, simulatorConnectorKind, trace.Line(), s.clk.Now()) //nolint:wrapcheck // The store names the failed write.
}

func (s *Service) readSimulatorLines(ctx context.Context, f io.Reader, trace *domain.SimulatorTrace, start int, batch *envelopeBatch) error {
	return transport.EachScannedLine(f, func(line []byte) error {
		trace.NextLine()
		if len(bytes.TrimSpace(line)) == 0 {
			return nil
		}
		env, isEvent, err := trace.Accept(line)
		if err != nil || !isEvent || trace.Line() <= start {
			return err //nolint:wrapcheck // The domain grammar names the failed line.
		}
		if err := batch.add(ctx, env); err != nil {
			return fmt.Errorf("append simulator batch: %w", err)
		}
		return nil
	})
}

package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/transport"
)

const jsonlConnectorKind = "jsonl-replay"

// jsonlRun is one pass over a trace file from a checkpoint.
type jsonlRun struct {
	connectorID string
	startLine   int
	batch       envelopeBatch
}

// ReplayJSONL reads the trace file from the connector's last checkpoint and
// appends all remaining envelopes. It returns the number of envelopes appended.
// The same file always appends the same events in the same order.
func (s *Service) ReplayJSONL(ctx context.Context, path, connectorID string) (int, error) {
	connectorID = domain.JSONLConnectorID(connectorID, path)
	f, err := transport.OpenTrace(path, "trace file")
	if err != nil {
		return 0, err //nolint:wrapcheck // The transport names the failed open.
	}
	defer func() { _ = f.Close() }()
	startLine, err := s.store.LoadLine(ctx, connectorID)
	if err != nil {
		return 0, fmt.Errorf("load checkpoint: %w", err)
	}
	run := jsonlRun{connectorID: connectorID, startLine: startLine, batch: envelopeBatch{log: s.log, tenantID: s.tenantID}}
	err = s.ingestAndCheckpoint(ctx, f, &run)
	return run.batch.appended, err
}

// ingestAndCheckpoint ingests the lines after the checkpoint, flushes the final
// batch and records the last line read.
func (s *Service) ingestAndCheckpoint(ctx context.Context, f io.Reader, run *jsonlRun) error {
	lineNum, err := s.ingestLines(ctx, f, run)
	if err != nil {
		return err
	}
	if err := run.batch.flush(ctx); err != nil {
		return err
	}
	if err := s.store.SaveLine(ctx, run.connectorID, jsonlConnectorKind, lineNum, s.clk.Now()); err != nil {
		return fmt.Errorf("save checkpoint: %w", err)
	}
	return nil
}

// ingestLines admits every line after the checkpoint into the batch and returns
// the number of lines read. A bounded reader caps per-line memory at the
// documented event size and lets an oversized line be quarantined and skipped
// rather than aborting the replay.
func (s *Service) ingestLines(ctx context.Context, f io.Reader, run *jsonlRun) (int, error) {
	reader := transport.NewBoundedReader(f, domain.MaxLineBytes)
	lineNum := 0
	for {
		line, tooLarge, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return lineNum, nil
		}
		if err != nil {
			return lineNum, fmt.Errorf("read trace file: %w", err)
		}
		lineNum++
		if err := s.ingestLine(ctx, line, lineNum, tooLarge, run); err != nil {
			return lineNum, err
		}
	}
}

// ingestLine admits a line after the checkpoint into the batch. The reader keeps
// the line terminator; it is stripped so payloads and quarantine records match
// the terminator-free line content.
func (s *Service) ingestLine(ctx context.Context, line []byte, lineNum int, tooLarge bool, run *jsonlRun) error {
	if lineNum <= run.startLine {
		return nil
	}
	env, admitted, err := s.admitLine(ctx, bytes.TrimRight(line, "\r\n"), run.connectorID, lineNum, tooLarge)
	if err != nil {
		return fmt.Errorf("quarantine line %d: %w", lineNum, err)
	}
	if !admitted {
		return nil
	}
	return run.batch.add(ctx, env)
}

// admitLine parses and validates one trace line. Blank lines are skipped, and an
// oversized, malformed, or invalid line is quarantined instead of aborting the
// replay; admitted reports whether env should be appended.
func (s *Service) admitLine(ctx context.Context, line []byte, connectorID string, lineNum int, tooLarge bool) (contractsv1.Envelope, bool, error) {
	now := s.clk.Now()
	if tooLarge {
		return contractsv1.Envelope{}, false, quarantined(domain.ReasonLineTooLarge, s.quarantineRaw(ctx, domain.QuarantineID(connectorID, lineNum), line, domain.ReasonLineTooLarge, now))
	}
	if len(bytes.TrimSpace(line)) == 0 {
		return contractsv1.Envelope{}, false, nil
	}
	verdict := s.admit(ctx, line)
	if !verdict.Rejected() {
		return verdict.Envelope, true, nil
	}
	return contractsv1.Envelope{}, false, quarantined(verdict.Reason, s.quarantineLine(ctx, verdict, line, connectorID, lineNum, now))
}

// quarantineLine records a rejected line, as its envelope when it decoded and as
// raw bytes otherwise.
func (s *Service) quarantineLine(ctx context.Context, verdict domain.LineVerdict, line []byte, connectorID string, lineNum int, now time.Time) error {
	if verdict.Decoded {
		return s.quarantineEnvelope(ctx, verdict.Envelope, verdict.Reason, now)
	}
	return s.quarantineRaw(ctx, domain.QuarantineID(connectorID, lineNum), line, verdict.Reason, now)
}

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

type jsonlRun struct {
	connectorID string
	startLine   int
	from        domain.Checkpoint
	batch       envelopeBatch
}

func (s *Service) ReplayJSONL(ctx context.Context, path, connectorID string) (int, error) {
	connectorID = domain.JSONLConnectorID(connectorID, path)
	position, err := s.store.LoadPosition(ctx, connectorID)
	if err != nil {
		return 0, fmt.Errorf("load checkpoint: %w", err)
	}
	f, resumed, err := transport.OpenTraceFrom(path, position.Offset)
	if err != nil {
		return 0, fmt.Errorf("replay %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	run := s.newJSONLRun(connectorID, position, resumed)
	err = s.ingestAndCheckpoint(ctx, f, &run)
	return run.batch.appended, err
}

func (s *Service) newJSONLRun(connectorID string, position domain.Checkpoint, resumed bool) jsonlRun {
	run := jsonlRun{connectorID: connectorID, startLine: position.LastLine, batch: envelopeBatch{log: s.log, tenantID: s.tenantID}}
	if resumed {
		run.from = position
	}
	return run
}

func (s *Service) ingestAndCheckpoint(ctx context.Context, f io.Reader, run *jsonlRun) error {
	reached, err := s.ingestLines(ctx, f, run)
	if err != nil {
		return err
	}
	if err := run.batch.flush(ctx); err != nil {
		return err
	}
	if err := s.store.SavePosition(ctx, run.connectorID, jsonlConnectorKind, reached, s.clk.Now()); err != nil {
		return fmt.Errorf("save checkpoint: %w", err)
	}
	return nil
}

func (s *Service) ingestLines(ctx context.Context, f io.Reader, run *jsonlRun) (domain.Checkpoint, error) {
	reader := transport.NewBoundedReader(f, domain.MaxLineBytes)
	lineNum := run.from.LastLine
	for {
		line, tooLarge, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return domain.Checkpoint{LastLine: lineNum, Offset: run.from.Offset + reader.Consumed()}, nil
		}
		if err != nil {
			return domain.Checkpoint{}, fmt.Errorf("read trace file: %w", err)
		}
		lineNum++
		if err := s.ingestLine(ctx, line, lineNum, tooLarge, run); err != nil {
			return domain.Checkpoint{}, err
		}
	}
}

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

func (s *Service) quarantineLine(ctx context.Context, verdict domain.LineVerdict, line []byte, connectorID string, lineNum int, now time.Time) error {
	if verdict.Decoded {
		return s.quarantineEnvelope(ctx, verdict.Envelope, verdict.Reason, now)
	}
	return s.quarantineRaw(ctx, domain.QuarantineID(connectorID, lineNum), line, verdict.Reason, now)
}

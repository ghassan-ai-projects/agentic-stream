// Package ingress provides connectors that read normalized events from external
// sources and append them to the event log.
package ingress

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// JSONLReplay reads a JSON Lines trace file and appends each envelope to the
// event log. It is deterministic: the same file always appends the same events
// in the same order and advances its connector checkpoint by line number.
type JSONLReplay struct {
	connectorID string
	tenantID    string
	path        string
	db          *storage.DB
	log         *eventlog.EventLog
	clk         clock.Clock
}

// NewJSONLReplay creates a JSONL replay connector.
func NewJSONLReplay(db *storage.DB, log *eventlog.EventLog, tenantID, path, connectorID string) *JSONLReplay {
	return NewJSONLReplayWithClock(db, log, tenantID, path, connectorID, clock.Physical())
}

// NewJSONLReplayWithClock creates a JSONL replay connector that uses clk for
// durable timestamps.
func NewJSONLReplayWithClock(db *storage.DB, log *eventlog.EventLog, tenantID, path, connectorID string, clk clock.Clock) *JSONLReplay {
	if connectorID == "" {
		connectorID = "jsonl:" + path
	}
	if clk == nil {
		clk = clock.Physical()
	}
	return &JSONLReplay{
		connectorID: connectorID,
		tenantID:    tenantID,
		path:        path,
		db:          db,
		log:         log,
		clk:         clk,
	}
}

// Run reads the trace file from the last checkpoint and appends all remaining
// envelopes. It returns the number of envelopes appended.
func (c *JSONLReplay) Run(ctx context.Context) (int, error) {
	f, err := os.Open(c.path)
	if err != nil {
		return 0, fmt.Errorf("open trace file: %w", err)
	}
	defer func() { _ = f.Close() }()

	startLine, err := c.loadCheckpoint(ctx)
	if err != nil {
		return 0, fmt.Errorf("load checkpoint: %w", err)
	}

	// A bounded reader (shared with the live-socket connector) caps per-line
	// memory at the documented event size and lets an oversized line be
	// quarantined and skipped rather than aborting the whole replay.
	reader := bufio.NewReaderSize(f, maxLiveSocketLineBytes)
	lineNum := 0
	appended := 0
	var batch []contractsv1.Envelope

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		positions, err := c.log.Append(ctx, c.tenantID, batch)
		if err != nil {
			return fmt.Errorf("append batch: %w", err)
		}
		for _, pos := range positions {
			if pos >= 0 {
				appended++
			}
		}
		batch = batch[:0]
		return nil
	}

	for {
		line, tooLarge, readErr := readBoundedLine(reader, maxLiveSocketLineBytes)
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return appended, fmt.Errorf("read trace file: %w", readErr)
		}
		// The reader keeps the line terminator; strip it so payloads and
		// quarantine records match the terminator-free line content.
		line = bytes.TrimRight(line, "\r\n")
		lineNum++
		if lineNum <= startLine {
			continue
		}
		if tooLarge {
			if quarantineErr := c.log.QuarantineRaw(ctx, c.tenantID, c.quarantineID(lineNum), line, "line_too_large", c.clk.Now().UTC().Format(time.RFC3339Nano)); quarantineErr != nil {
				return appended, fmt.Errorf("quarantine line %d: %w", lineNum, quarantineErr)
			}
			continue
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var env contractsv1.Envelope
		if err := json.Unmarshal(line, &env); err != nil {
			if quarantineErr := c.log.QuarantineRaw(ctx, c.tenantID, c.quarantineID(lineNum), line, "malformed_json", c.clk.Now().UTC().Format(time.RFC3339Nano)); quarantineErr != nil {
				return appended, fmt.Errorf("quarantine line %d: %w", lineNum, quarantineErr)
			}
			continue
		}
		if env.TenantID == "" {
			env.TenantID = c.tenantID
		}
		if err := contractsv1.ValidateEnvelope(env, c.tenantID); err != nil {
			if quarantineErr := c.log.QuarantineEnvelope(ctx, c.tenantID, env, "envelope_invalid", c.clk.Now().UTC().Format(time.RFC3339Nano)); quarantineErr != nil {
				return appended, fmt.Errorf("quarantine line %d: %w", lineNum, quarantineErr)
			}
			continue
		}
		if err := c.log.ValidateEnvelope(ctx, env); err != nil {
			if quarantineErr := c.log.QuarantineEnvelope(ctx, c.tenantID, env, "schema_invalid", c.clk.Now().UTC().Format(time.RFC3339Nano)); quarantineErr != nil {
				return appended, fmt.Errorf("quarantine line %d: %w", lineNum, quarantineErr)
			}
			continue
		}
		batch = append(batch, env)
		if len(batch) >= 100 {
			if err := flush(); err != nil {
				return appended, err
			}
		}
	}
	if err := flush(); err != nil {
		return appended, err
	}

	if err := c.saveCheckpoint(ctx, lineNum); err != nil {
		return appended, fmt.Errorf("save checkpoint: %w", err)
	}

	return appended, nil
}

// quarantineID scopes a raw-line quarantine record to this connector so two
// traces ingested by the same tenant cannot collide on (tenant_id, event_id).
// The quarantine table treats a same-ID/different-payload write as a conflict
// (marking the prior record rejected and erroring), so a bare "line:<N>" would
// let a second trace's malformed line at the same line number abort ingestion
// and corrupt the first record.
func (c *JSONLReplay) quarantineID(lineNum int) string {
	return fmt.Sprintf("%s:line:%d", c.connectorID, lineNum)
}

// readBoundedLine reads one newline-terminated line, capping memory at max
// bytes. When a line exceeds max it returns the truncated prefix with
// tooLarge=true and resynchronizes to the start of the next line (discarding
// the overlong remainder) so ingestion continues rather than aborting. It
// returns io.EOF only when no bytes remained to read. Unlike the live-socket
// reader, which drops its connection after an oversized frame, this resyncs
// deterministically for a file trace.
func readBoundedLine(r *bufio.Reader, max int) ([]byte, bool, error) {
	var line []byte
	over := false
	for {
		part, err := r.ReadSlice('\n')
		if len(line)+len(part) > max {
			over = true
			if room := max - len(line); room > 0 {
				line = append(line, part[:room]...)
			}
		} else {
			line = append(line, part...)
		}
		switch {
		case err == nil:
			// The newline was found and consumed; the line is complete.
			return line, over, nil
		case errors.Is(err, bufio.ErrBufferFull):
			if over {
				if derr := discardToNewline(r); derr != nil && !errors.Is(derr, io.EOF) {
					return line, true, derr
				}
				return line, true, nil
			}
			continue
		case errors.Is(err, io.EOF):
			if len(line) > 0 || over {
				return line, over, nil
			}
			return nil, false, io.EOF
		default:
			return line, over, fmt.Errorf("read line: %w", err)
		}
	}
}

// discardToNewline consumes the reader up to and including the next newline,
// used to resynchronize after an oversized line whose prefix was quarantined.
func discardToNewline(r *bufio.Reader) error {
	for {
		_, err := r.ReadSlice('\n')
		if err == nil {
			return nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return err
	}
}

func (c *JSONLReplay) loadCheckpoint(ctx context.Context) (int, error) {
	var checkpointBlob []byte
	if err := c.db.QueryRowContext(ctx,
		"SELECT checkpoint_blob FROM connector_checkpoints WHERE connector_id = ?",
		c.connectorID,
	).Scan(&checkpointBlob); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No checkpoint is equivalent to starting at line 0.
			return 0, nil
		}
		return 0, fmt.Errorf("load connector checkpoint: %w", err)
	}
	var cp checkpoint
	if err := json.Unmarshal(checkpointBlob, &cp); err != nil {
		return 0, fmt.Errorf("unmarshal checkpoint: %w", err)
	}
	return cp.LastLine, nil
}

func (c *JSONLReplay) saveCheckpoint(ctx context.Context, lastLine int) error {
	cp := checkpoint{LastLine: lastLine, Version: 1}
	blob, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	now := c.clk.Now().UTC().Format(time.RFC3339Nano)
	if _, err := c.db.ExecContext(ctx, `
		INSERT INTO connector_checkpoints (connector_id, connector_kind, checkpoint_version, checkpoint_blob, updated_at)
		VALUES (?, 'jsonl-replay', ?, ?, ?)
		ON CONFLICT(connector_id)
		DO UPDATE SET checkpoint_version = excluded.checkpoint_version,
		              checkpoint_blob = excluded.checkpoint_blob,
		              updated_at = excluded.updated_at`,
		c.connectorID, cp.Version, blob, now,
	); err != nil {
		return fmt.Errorf("upsert checkpoint: %w", err)
	}
	return nil
}

type checkpoint struct {
	Version  int `json:"version"`
	LastLine int `json:"last_line"`
}

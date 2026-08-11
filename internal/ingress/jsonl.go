// Package ingress provides connectors that read normalized events from external
// sources and append them to the event log.
package ingress

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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

	scanner := bufio.NewScanner(f)
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

	for scanner.Scan() {
		lineNum++
		if lineNum <= startLine {
			continue
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var env contractsv1.Envelope
		if err := json.Unmarshal(line, &env); err != nil {
			return appended, fmt.Errorf("parse line %d: %w", lineNum, err)
		}
		if env.TenantID == "" {
			env.TenantID = c.tenantID
		}
		batch = append(batch, env)
		if len(batch) >= 100 {
			if err := flush(); err != nil {
				return appended, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return appended, fmt.Errorf("read trace file: %w", err)
	}
	if err := flush(); err != nil {
		return appended, err
	}

	if err := c.saveCheckpoint(ctx, lineNum); err != nil {
		return appended, fmt.Errorf("save checkpoint: %w", err)
	}

	return appended, nil
}

func (c *JSONLReplay) loadCheckpoint(ctx context.Context) (int, error) {
	var checkpointBlob []byte
	if err := c.db.QueryRowContext(ctx,
		"SELECT checkpoint_blob FROM connector_checkpoints WHERE connector_id = ?",
		c.connectorID,
	).Scan(&checkpointBlob); err != nil {
		// No checkpoint is equivalent to starting at line 0.
		return 0, nil
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

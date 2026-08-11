// Package engine runs the deterministic stream processing loop.
package engine

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// ConsumerName identifies the engine's inbox consumer.
const ConsumerName = "engine"

// Engine processes events for a single virtual partition deterministically.
type Engine struct {
	db       *storage.DB
	log      *eventlog.EventLog
	clock    clock.Clock
	spec     *spec.CompiledSpec
	tenantID string
}

// NewEngine creates an engine for the given spec and tenant.
func NewEngine(db *storage.DB, log *eventlog.EventLog, clk clock.Clock, spec *spec.CompiledSpec, tenantID string) *Engine {
	if tenantID == "" {
		tenantID = contractsv1.TenantID
	}
	return &Engine{
		db:       db,
		log:      log,
		clock:    clk,
		spec:     spec,
		tenantID: tenantID,
	}
}

// Run reads and applies events for partitionID until no more unprocessed
// records remain. It returns the number of events processed.
func (e *Engine) Run(ctx context.Context, partitionID int) (int, error) {
	processed := 0
	for {
		n, err := e.runBatch(ctx, partitionID)
		if err != nil {
			return processed, err
		}
		processed += n
		if n == 0 {
			break
		}
	}
	return processed, nil
}

func (e *Engine) runBatch(ctx context.Context, partitionID int) (int, error) {
	checkpoint, err := e.loadCheckpoint(ctx, partitionID)
	if err != nil {
		return 0, fmt.Errorf("load checkpoint: %w", err)
	}

	const batchSize = 100
	var records []eventlog.Record
	err = e.log.Read(ctx, eventlog.ReadRequest{
		TenantID:      e.tenantID,
		PartitionID:   partitionID,
		AfterPosition: checkpoint.LastPosition,
		Limit:         batchSize,
	}, func(r eventlog.Record) error {
		records = append(records, r)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("read event log: %w", err)
	}
	if len(records) == 0 {
		return 0, nil
	}

	for _, rec := range records {
		if err := e.applyRecord(ctx, partitionID, rec, checkpoint); err != nil {
			return 0, fmt.Errorf("apply record %d: %w", rec.Position, err)
		}
	}

	return len(records), nil
}

type checkpoint struct {
	LastPosition eventlog.LogPosition
	Watermark    string
}

func (e *Engine) loadCheckpoint(ctx context.Context, partitionID int) (checkpoint, error) {
	var cp checkpoint
	var lastPosition int64
	var watermark sql.NullString
	if err := e.db.QueryRowContext(ctx,
		"SELECT last_position, watermark FROM partition_checkpoints WHERE consumer_name = ? AND tenant_id = ? AND partition_id = ?",
		ConsumerName, e.tenantID, partitionID,
	).Scan(&lastPosition, &watermark); err != nil && err != sql.ErrNoRows {
		return cp, fmt.Errorf("query checkpoint: %w", err)
	}
	cp.LastPosition = eventlog.LogPosition(lastPosition)
	if watermark.Valid {
		cp.Watermark = watermark.String
	}
	return cp, nil
}

func (e *Engine) applyRecord(ctx context.Context, partitionID int, rec eventlog.Record, cp checkpoint) error {
	if err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		// Idempotency: skip if already applied.
		var applied bool
		if err := tx.QueryRowContext(ctx,
			"SELECT 1 FROM event_inbox WHERE consumer_name = ? AND tenant_id = ? AND event_id = ?",
			ConsumerName, e.tenantID, rec.EventID,
		).Scan(&applied); err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("check inbox: %w", err)
		}
		if applied {
			return nil
		}

		// TODO: run operators and situation reducer here.

		// Advance checkpoint.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO partition_checkpoints (consumer_name, tenant_id, partition_id, last_position, watermark, updated_at)
			VALUES (?, ?, ?, ?, ?, datetime('now'))
			ON CONFLICT(consumer_name, tenant_id, partition_id)
			DO UPDATE SET last_position = excluded.last_position, updated_at = excluded.updated_at`,
			ConsumerName, e.tenantID, partitionID, int64(rec.Position), cp.Watermark,
		); err != nil {
			return fmt.Errorf("update checkpoint: %w", err)
		}

		// Mark inbox applied.
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO event_inbox (consumer_name, tenant_id, event_id, log_position, applied_at) VALUES (?, ?, ?, ?, datetime('now'))",
			ConsumerName, e.tenantID, rec.EventID, int64(rec.Position),
		); err != nil {
			return fmt.Errorf("mark inbox: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("apply record transaction: %w", err)
	}
	return nil
}

package engine

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func (e *Engine) applyRecord(ctx context.Context, partitionID int, record eventlog.Record, watermark time.Time) error {
	err := storage.RetrySQLiteBusy(ctx, func() error {
		return e.applyRecordTransaction(ctx, partitionID, record, watermark)
	})
	if err != nil {
		return fmt.Errorf("apply record transaction: %w", err)
	}
	return nil
}

func (e *Engine) applyRecordTransaction(ctx context.Context, partitionID int, record eventlog.Record, watermark time.Time) error {
	err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := e.assertOwner(ctx, tx); err != nil {
			return err
		}
		applied, err := e.eventAlreadyApplied(ctx, tx, record.EventID)
		if err != nil {
			return err
		}
		if applied {
			return nil
		}
		operatorState, err := e.loadOperatorState(ctx, tx, partitionID, record.Envelope.Entity.ID)
		if err != nil {
			return fmt.Errorf("load operator state: %w", err)
		}
		features, newState, err := e.opRuntime.ApplyEventAt(ctx, operatorState, record.Envelope, watermark, e.clock.Now().UTC())
		if err != nil {
			return fmt.Errorf("apply operators: %w", err)
		}
		affected, err := e.applyFeatures(ctx, tx, partitionID, features, watermark)
		if err != nil {
			return err
		}
		if err := e.saveAffectedSituationStates(ctx, tx, partitionID, affected); err != nil {
			return err
		}
		if err := e.saveOperatorState(ctx, tx, partitionID, record.Envelope.Entity.ID, newState); err != nil {
			return fmt.Errorf("save operator state: %w", err)
		}
		if err := e.scheduleHeartbeatTimers(ctx, tx, partitionID, newState); err != nil {
			return fmt.Errorf("schedule heartbeat timers: %w", err)
		}
		return e.commitRecord(ctx, tx, partitionID, record, watermark)
	})
	if err == nil {
		return nil
	}
	e.sitEngine.Reset()
	if restoreErr := restoreSituations(ctx, e.db, e.deploymentID, e.tenantID, e.sitEngine); restoreErr != nil {
		return fmt.Errorf("apply record transaction: %w; restore after rollback: %w", err, restoreErr)
	}
	return fmt.Errorf("apply transaction: %w", err)
}

func (e *Engine) eventAlreadyApplied(ctx context.Context, tx *sql.Tx, eventID string) (bool, error) {
	var applied bool
	if err := tx.QueryRowContext(ctx,
		"SELECT 1 FROM event_inbox WHERE consumer_name = ? AND tenant_id = ? AND event_id = ?",
		ConsumerName, e.tenantID, eventID,
	).Scan(&applied); err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("check inbox: %w", err)
	}
	return applied, nil
}

func (e *Engine) applyFeatures(ctx context.Context, tx *sql.Tx, partitionID int, features []operators.Feature, watermark time.Time) (map[string]struct{}, error) {
	affected := make(map[string]struct{})
	for _, feature := range features {
		feature.TenantID = e.tenantID
		feature.PartitionID = partitionID
		affected[feature.EntityType+"\x00"+feature.EntityID] = struct{}{}
		versions, err := e.sitEngine.ApplyFeature(ctx, feature, watermark)
		if err != nil {
			return nil, fmt.Errorf("apply situation: %w", err)
		}
		for _, version := range versions {
			if err := e.saveSituationVersion(ctx, tx, partitionID, version); err != nil {
				return nil, fmt.Errorf("save situation version: %w", err)
			}
			if e.cogEngine != nil {
				if err := e.cogEngine.Process(ctx, tx, version); err != nil {
					return nil, fmt.Errorf("cognition process: %w", err)
				}
			}
		}
	}
	return affected, nil
}

func (e *Engine) saveAffectedSituationStates(ctx context.Context, tx *sql.Tx, partitionID int, affected map[string]struct{}) error {
	for key := range affected {
		parts := strings.SplitN(key, "\x00", 2)
		situation, stateJSON, stateDigest, ok, err := e.sitEngine.CurrentState(partitionID, parts[0], parts[1])
		if err != nil {
			return fmt.Errorf("snapshot current situation state: %w", err)
		}
		if ok && situation.Version > 0 {
			if err := e.saveSituationRuntimeState(ctx, tx, situation, stateJSON, stateDigest); err != nil {
				return fmt.Errorf("save current situation state: %w", err)
			}
		}
	}
	return nil
}

func (e *Engine) commitRecord(ctx context.Context, tx *sql.Tx, partitionID int, record eventlog.Record, watermark time.Time) error {
	now := e.clock.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO partition_checkpoints (consumer_name, tenant_id, partition_id, last_position, watermark, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(consumer_name, tenant_id, partition_id)
		DO UPDATE SET last_position = excluded.last_position, watermark = excluded.watermark, updated_at = excluded.updated_at`,
		ConsumerName, e.tenantID, partitionID, int64(record.Position), watermark.Format(time.RFC3339Nano), now); err != nil {
		return fmt.Errorf("update checkpoint: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO event_inbox (consumer_name, tenant_id, event_id, log_position, applied_at) VALUES (?, ?, ?, ?, ?)",
		ConsumerName, e.tenantID, record.EventID, int64(record.Position), now); err != nil {
		return fmt.Errorf("mark inbox: %w", err)
	}
	return nil
}

func (e *Engine) assertOwner(ctx context.Context, tx *sql.Tx) error {
	if e.owner == nil || e.ownerEpoch == "" {
		return nil
	}
	if err := e.owner.Assert(ctx, tx, e.ownerEpoch); err != nil {
		return fmt.Errorf("stream runtime ownership lost: %w", err)
	}
	return nil
}

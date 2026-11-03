package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
)

func (tx *Tx) restoreWindowSamples(ctx context.Context, partitionID int, entityID string, state *operators.PartitionState) error {
	samples, err := tx.loadWindowSamples(ctx, partitionID, entityID)
	if err != nil {
		return err
	}
	for ref, held := range samples {
		if window := windowOf(state, ref); window != nil {
			window.RestoreSamples(held)
		}
	}
	return nil
}

func windowOf(state *operators.PartitionState, ref operatorStateRef) *operators.WindowState {
	if blob := state.OperatorStates[ref.operatorID][ref.stateKey]; blob != nil {
		return blob.Window
	}
	return nil
}

func (tx *Tx) loadWindowSamples(ctx context.Context, partitionID int, entityID string) (map[operatorStateRef][]operators.Sample, error) {
	args := append([]any{tx.deploymentID, tx.tenantID, partitionID, entityID, entityID}, entityScopeArgs(entityID)...)
	rows, err := tx.tx.QueryContext(ctx, windowSamplesSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query window samples: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanWindowSamples(rows)
}

const windowSamplesSQL = `
		SELECT operator_id, state_key, event_time, event_id, value, boot_id FROM operator_window_samples
		WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
		  AND state_key >= ? AND state_key < ? || char(32)
		  AND ` + entityScopePredicate

func scanWindowSamples(rows *sql.Rows) (map[operatorStateRef][]operators.Sample, error) {
	samples := make(map[operatorStateRef][]operators.Sample)
	for rows.Next() {
		ref, sample, err := scanWindowSample(rows)
		if err != nil {
			return nil, err
		}
		samples[ref] = append(samples[ref], sample)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate window samples: %w", err)
	}
	return samples, nil
}

func scanWindowSample(rows *sql.Rows) (operatorStateRef, operators.Sample, error) {
	var ref operatorStateRef
	var sample operators.Sample
	var eventTime string
	if err := rows.Scan(&ref.operatorID, &ref.stateKey, &eventTime, &sample.EventID, &sample.Value, &sample.BootID); err != nil {
		return ref, sample, fmt.Errorf("scan window sample: %w", err)
	}
	at, err := kernel.ParseStoredTime(eventTime)
	if err != nil {
		return ref, sample, fmt.Errorf("window sample %s: %w", sample.EventID, err)
	}
	sample.EventTime = at
	return ref, sample, nil
}

func (tx *Tx) writeWindowSampleChanges(ctx context.Context, partitionID int, state *operators.PartitionState) error {
	for operatorID, states := range state.OperatorStates {
		for stateKey, blob := range states {
			if blob.Window == nil {
				continue
			}
			if err := tx.writeSampleChanges(ctx, partitionID, operatorStateRef{operatorID, stateKey}, blob.Window); err != nil {
				return err
			}
		}
	}
	return nil
}

func (tx *Tx) writeSampleChanges(ctx context.Context, partitionID int, ref operatorStateRef, window *operators.WindowState) error {
	admitted, expired := window.UnsavedSamples()
	for _, sample := range expired {
		if err := tx.deleteWindowSample(ctx, partitionID, ref, sample); err != nil {
			return err
		}
	}
	for _, sample := range admitted {
		if err := tx.insertWindowSample(ctx, partitionID, ref, sample); err != nil {
			return err
		}
	}
	window.SamplesSaved()
	return nil
}

func (tx *Tx) insertWindowSample(ctx context.Context, partitionID int, ref operatorStateRef, sample operators.Sample) error {
	if _, err := tx.tx.ExecContext(ctx, `
		INSERT INTO operator_window_samples (
			deployment_id, tenant_id, partition_id, operator_id, state_key, event_time, event_id, value, boot_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tx.deploymentID, tx.tenantID, partitionID, ref.operatorID, ref.stateKey,
		kernel.FormatTime(sample.EventTime), sample.EventID, sample.Value, sample.BootID); err != nil {
		return fmt.Errorf("insert window sample %s of %s/%s: %w", sample.EventID, ref.operatorID, ref.stateKey, err)
	}
	return nil
}

func (tx *Tx) deleteWindowSample(ctx context.Context, partitionID int, ref operatorStateRef, sample operators.Sample) error {
	if _, err := tx.tx.ExecContext(ctx, `
		DELETE FROM operator_window_samples
		WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ? AND operator_id = ? AND state_key = ?
		  AND event_time = ? AND event_id = ?`,
		tx.deploymentID, tx.tenantID, partitionID, ref.operatorID, ref.stateKey,
		kernel.FormatTime(sample.EventTime), sample.EventID); err != nil {
		return fmt.Errorf("expire window sample %s of %s/%s: %w", sample.EventID, ref.operatorID, ref.stateKey, err)
	}
	return nil
}

func (tx *Tx) deleteWindowSamples(ctx context.Context, partitionID int, ref operatorStateRef) error {
	if _, err := tx.tx.ExecContext(ctx, `
		DELETE FROM operator_window_samples
		WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ? AND operator_id = ? AND state_key = ?`,
		tx.deploymentID, tx.tenantID, partitionID, ref.operatorID, ref.stateKey); err != nil {
		return fmt.Errorf("retire window samples of %s/%s: %w", ref.operatorID, ref.stateKey, err)
	}
	return nil
}

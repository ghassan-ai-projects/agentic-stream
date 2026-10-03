package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
)

func (e *Engine) runDueTimersForAllPartitions(ctx context.Context) (int, error) {
	partitions, err := e.timerPartitions(ctx)
	if err != nil {
		return 0, err
	}

	var fired int
	for _, partitionID := range partitions {
		partitionFired, err := e.runDueTimers(ctx, partitionID)
		if err != nil {
			return fired, err
		}
		fired += partitionFired
	}
	return fired, nil
}

func (e *Engine) timerPartitions(ctx context.Context) ([]int, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT DISTINCT partition_id FROM timers
		WHERE deployment_id = ? AND tenant_id = ? AND timer_kind = 'processing_time' AND status = 'pending'
		ORDER BY partition_id`, e.deploymentID, e.tenantID)
	if err != nil {
		return nil, fmt.Errorf("query timer partitions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanTimerPartitions(rows)
}

func scanTimerPartitions(rows *sql.Rows) ([]int, error) {
	var partitions []int
	for rows.Next() {
		var partitionID int
		if err := rows.Scan(&partitionID); err != nil {
			return nil, fmt.Errorf("scan timer partition: %w", err)
		}
		partitions = append(partitions, partitionID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate timer partitions: %w", err)
	}
	return partitions, nil
}

func (e *Engine) runDueTimers(ctx context.Context, partitionID int) (int, error) {
	now := e.clock.Now().UTC()
	watermark, err := e.timerWatermark(ctx, partitionID, now)
	if err != nil {
		return 0, err
	}
	fired := 0
	if err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		fired, err = e.fireDueTimers(ctx, tx, partitionID, watermark, now)
		return err
	}); err != nil {
		return 0, e.restoreAfterTimerFailure(ctx, err)
	}
	return fired, nil
}

// fireDueTimers applies the partition's due timers under the owner fence.
func (e *Engine) fireDueTimers(ctx context.Context, tx *sql.Tx, partitionID int, watermark, now time.Time) (int, error) {
	if err := e.assertOwner(ctx, tx); err != nil {
		return 0, err
	}
	timers, err := e.loadDueTimers(ctx, tx, partitionID, now)
	if err != nil || len(timers) == 0 {
		return 0, err
	}
	return e.applyDueTimers(ctx, tx, partitionID, timers, watermark, now)
}

func (e *Engine) timerWatermark(ctx context.Context, partitionID int, now time.Time) (time.Time, error) {
	checkpoint, err := e.loadCheckpoint(ctx, partitionID)
	if err != nil {
		return time.Time{}, fmt.Errorf("load timer checkpoint: %w", err)
	}
	if checkpoint.Watermark == "" {
		return now, nil
	}
	watermark, err := time.Parse(time.RFC3339Nano, checkpoint.Watermark)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timer watermark: %w", err)
	}
	return watermark, nil
}

type dueTimer struct {
	id, operatorID, stateKey, dueAt, expectedEventID string
}

func (e *Engine) loadDueTimers(ctx context.Context, tx *sql.Tx, partitionID int, now time.Time) ([]dueTimer, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT timer_id, operator_id, state_key, due_at, payload_json FROM timers
		WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
		  AND timer_kind = 'processing_time' AND status = 'pending' AND due_at <= ?
		ORDER BY due_at, timer_id`,
		e.deploymentID, e.tenantID, partitionID, now.Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("query due timers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanDueTimers(rows)
}

func scanDueTimers(rows *sql.Rows) ([]dueTimer, error) {
	var timers []dueTimer
	for rows.Next() {
		timer, err := scanDueTimer(rows)
		if err != nil {
			return nil, err
		}
		timers = append(timers, timer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due timers: %w", err)
	}
	return timers, nil
}

// scanDueTimer reads one timer and the event its payload expects to be last.
func scanDueTimer(rows *sql.Rows) (dueTimer, error) {
	var timer dueTimer
	var payload []byte
	if err := rows.Scan(&timer.id, &timer.operatorID, &timer.stateKey, &timer.dueAt, &payload); err != nil {
		return dueTimer{}, fmt.Errorf("scan due timer: %w", err)
	}
	var timerPayload struct {
		ExpectedEventID string `json:"expected_event_id"`
	}
	if err := json.Unmarshal(payload, &timerPayload); err != nil {
		return dueTimer{}, fmt.Errorf("decode timer payload %s: %w", timer.id, err)
	}
	timer.expectedEventID = timerPayload.ExpectedEventID
	return timer, nil
}

func (e *Engine) applyDueTimers(ctx context.Context, tx *sql.Tx, partitionID int, timers []dueTimer, watermark, now time.Time) (int, error) {
	state, features, err := e.timerFeatures(ctx, tx, partitionID, watermark, now)
	if err != nil {
		return 0, err
	}
	appliedFeatures, err := e.applyMatchedTimerFeatures(ctx, tx, partitionID, state, timers, features, watermark, now)
	if err != nil {
		return 0, err
	}
	if err := e.saveTimerSituationStates(ctx, tx, partitionID, appliedFeatures); err != nil {
		return 0, err
	}
	return len(timers), e.acknowledgeTimers(ctx, tx, timers, now)
}

// timerFeatures loads the partition's operator state and lets the operators
// emit their timer features.
func (e *Engine) timerFeatures(ctx context.Context, tx *sql.Tx, partitionID int, watermark, now time.Time) (*operators.PartitionState, []operators.Feature, error) {
	state, err := e.loadOperatorStateForPartition(ctx, tx, partitionID)
	if err != nil {
		return nil, nil, err
	}
	features, _, err := e.opRuntime.ApplyTimer(ctx, state, watermark, now)
	if err != nil {
		return nil, nil, fmt.Errorf("apply timers: %w", err)
	}
	return state, features, nil
}

func (e *Engine) saveTimerSituationStates(ctx context.Context, tx *sql.Tx, partitionID int, features []operators.Feature) error {
	updated := make(map[string]struct{}, len(features))
	for _, feature := range features {
		key := feature.EntityType + "\x00" + feature.EntityID
		if _, seen := updated[key]; seen {
			continue
		}
		updated[key] = struct{}{}
		if err := e.saveCurrentSituationState(ctx, tx, partitionID, feature.EntityType, feature.EntityID); err != nil {
			return err
		}
	}
	return nil
}

// saveCurrentSituationState persists the entity's in-memory Situation state
// once it has published a version.
func (e *Engine) saveCurrentSituationState(ctx context.Context, tx *sql.Tx, partitionID int, entityType, entityID string) error {
	situation, stateJSON, stateDigest, ok, err := e.sitEngine.CurrentState(partitionID, entityType, entityID)
	if err != nil {
		return fmt.Errorf("snapshot current situation state: %w", err)
	}
	if !ok || situation.Version <= 0 {
		return nil
	}
	if err := e.saveSituationRuntimeState(ctx, tx, situation, stateJSON, stateDigest); err != nil {
		return fmt.Errorf("save current situation state: %w", err)
	}
	return nil
}

func (e *Engine) acknowledgeTimers(ctx context.Context, tx *sql.Tx, timers []dueTimer, now time.Time) error {
	placeholders := strings.TrimRight(strings.Repeat("?,", len(timers)), ",")
	args := make([]any, 0, len(timers)+2)
	args = append(args, now.Format(time.RFC3339Nano))
	for _, timer := range timers {
		args = append(args, timer.id)
	}
	args = append(args, e.tenantID, e.deploymentID)
	//nolint:gosec // placeholders are generated from timer count, never user input.
	query := fmt.Sprintf(`UPDATE timers SET status = 'fired', fired_at = ?
		WHERE timer_id IN (%s) AND tenant_id = ? AND deployment_id = ? AND status = 'pending'`, placeholders)
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("acknowledge timers: %w", err)
	}
	return nil
}

func (e *Engine) restoreAfterTimerFailure(ctx context.Context, err error) error {
	e.sitEngine.Reset()
	if restoreErr := restoreSituations(ctx, e.db, e.deploymentID, e.tenantID, e.sitEngine); restoreErr != nil {
		return fmt.Errorf("run timers transaction: %w; restore after rollback: %w", err, restoreErr)
	}
	return fmt.Errorf("run timers transaction: %w", err)
}

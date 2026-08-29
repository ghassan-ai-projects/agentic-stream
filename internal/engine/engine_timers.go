package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
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
		if err := e.assertOwner(ctx, tx); err != nil {
			return err
		}

		timers, err := e.loadDueTimers(ctx, tx, partitionID, now)
		if err != nil {
			return err
		}
		if len(timers) == 0 {
			return nil
		}

		fired, err = e.applyDueTimers(ctx, tx, partitionID, timers, watermark, now)
		return err
	}); err != nil {
		return 0, e.restoreAfterTimerFailure(ctx, err)
	}
	return fired, nil
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

	var timers []dueTimer
	for rows.Next() {
		var timer dueTimer
		var payload []byte
		if err := rows.Scan(&timer.id, &timer.operatorID, &timer.stateKey, &timer.dueAt, &payload); err != nil {
			return nil, fmt.Errorf("scan due timer: %w", err)
		}
		var timerPayload struct {
			ExpectedEventID string `json:"expected_event_id"`
		}
		if err := json.Unmarshal(payload, &timerPayload); err != nil {
			return nil, fmt.Errorf("decode timer payload %s: %w", timer.id, err)
		}
		timer.expectedEventID = timerPayload.ExpectedEventID
		timers = append(timers, timer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due timers: %w", err)
	}
	return timers, nil
}

func (e *Engine) applyDueTimers(ctx context.Context, tx *sql.Tx, partitionID int, timers []dueTimer, watermark, now time.Time) (int, error) {
	state, err := e.loadOperatorStateForPartition(ctx, tx, partitionID)
	if err != nil {
		return 0, err
	}
	features, _, err := e.opRuntime.ApplyTimer(ctx, state, watermark, now)
	if err != nil {
		return 0, fmt.Errorf("apply timers: %w", err)
	}

	timersByState := indexTimers(timers)
	matched := activeTimerIDs(e.opRuntime, state, timers)
	var appliedFeatures []operators.Feature
	for _, feature := range features {
		timer, ok := timersByState[feature.OperatorID+"\x00"+timerStateKey(feature)]
		if !ok || !matchesExpectedEvent(feature, timer.expectedEventID) {
			continue
		}
		matched[timer.id] = struct{}{}
		enrichTimerFeature(&feature, e.tenantID, partitionID, timer, now, e.clock)
		appliedFeatures = append(appliedFeatures, feature)
		if err := e.saveTimerFeature(ctx, tx, partitionID, feature, watermark); err != nil {
			return 0, err
		}
	}
	if len(matched) != len(timers) {
		return 0, fmt.Errorf("due timer has no matching operator state: matched %d of %d", len(matched), len(timers))
	}
	if err := e.saveTimerSituationStates(ctx, tx, partitionID, appliedFeatures); err != nil {
		return 0, err
	}
	if err := e.acknowledgeTimers(ctx, tx, timers, now); err != nil {
		return 0, err
	}
	return len(timers), nil
}

func indexTimers(timers []dueTimer) map[string]dueTimer {
	byState := make(map[string]dueTimer, len(timers))
	for _, timer := range timers {
		byState[timer.operatorID+"\x00"+timer.stateKey] = timer
	}
	return byState
}

func activeTimerIDs(runtime *operators.OperatorRuntime, state *operators.PartitionState, timers []dueTimer) map[string]struct{} {
	matched := make(map[string]struct{}, len(timers))
	for _, timer := range timers {
		if !runtime.IsTimerStateActive(state, timer.stateKey) {
			// Boot fencing intentionally suppresses stale timers so they cannot
			// block the partition forever.
			matched[timer.id] = struct{}{}
		}
	}
	return matched
}

func timerStateKey(feature operators.Feature) string {
	if feature.StateKey != "" {
		return feature.StateKey
	}
	return feature.EntityID
}

func matchesExpectedEvent(feature operators.Feature, expectedEventID string) bool {
	if len(feature.InputEventIDs) == 0 {
		return false
	}
	return feature.InputEventIDs[len(feature.InputEventIDs)-1] == expectedEventID
}

func enrichTimerFeature(feature *operators.Feature, tenantID string, partitionID int, timer dueTimer, now time.Time, clk clock.Clock) {
	feature.TenantID = tenantID
	feature.PartitionID = partitionID
	feature.Metadata = map[string]any{
		"timer_id":               timer.id,
		"timer_basis":            "processing_time",
		"timer_due_at":           timer.dueAt,
		"timer_fired_at":         now.Format(time.RFC3339Nano),
		"expected_event_horizon": timer.dueAt,
		"clock_quality":          clock.Quality(clk),
		"source_traceparent":     feature.Traceparent,
		"source_tracestate":      feature.Tracestate,
	}
}

func (e *Engine) saveTimerFeature(ctx context.Context, tx *sql.Tx, partitionID int, feature operators.Feature, watermark time.Time) error {
	versions, err := e.sitEngine.ApplyFeature(ctx, feature, watermark)
	if err != nil {
		return fmt.Errorf("apply timer situation: %w", err)
	}
	for _, version := range versions {
		if err := e.saveSituationVersion(ctx, tx, partitionID, version); err != nil {
			return fmt.Errorf("save timer situation version: %w", err)
		}
		if e.cogEngine != nil {
			if err := e.cogEngine.Process(ctx, tx, version); err != nil {
				return fmt.Errorf("process timer cognition: %w", err)
			}
		}
	}
	return nil
}

func (e *Engine) saveTimerSituationStates(ctx context.Context, tx *sql.Tx, partitionID int, features []operators.Feature) error {
	updated := make(map[string]struct{}, len(features))
	for _, feature := range features {
		key := feature.EntityType + "\x00" + feature.EntityID
		if _, seen := updated[key]; seen {
			continue
		}
		updated[key] = struct{}{}
		situation, stateJSON, stateDigest, ok, err := e.sitEngine.CurrentState(partitionID, feature.EntityType, feature.EntityID)
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

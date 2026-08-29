package engine

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/duration"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func (e *Engine) scheduleHeartbeatTimers(ctx context.Context, tx *sql.Tx, partitionID int, state *operators.PartitionState) error {
	if state == nil {
		return nil
	}
	now := e.clock.Now().UTC().Format(time.RFC3339Nano)
	for _, operator := range e.spec.Operators {
		if operator.Kind != "missing_heartbeat" {
			continue
		}
		if err := e.scheduleOperatorHeartbeatTimers(ctx, tx, partitionID, state, operator, now); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) scheduleOperatorHeartbeatTimers(ctx context.Context, tx *sql.Tx, partitionID int, state *operators.PartitionState, operator spec.Operator, now string) error {
	delay, err := duration.Parse(operator.Duration)
	if err != nil {
		return fmt.Errorf("parse %s duration: %w", operator.Name, err)
	}
	for stateKey, blob := range state.OperatorStates[operator.Name] {
		if blob == nil || blob.Heartbeat == nil || blob.Heartbeat.LastEventTime == nil {
			continue
		}
		if err := e.scheduleHeartbeatTimer(ctx, tx, partitionID, operator, stateKey, blob, delay, now); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) scheduleHeartbeatTimer(ctx context.Context, tx *sql.Tx, partitionID int, operator spec.Operator, stateKey string, blob *operators.OperatorStateBlob, delay time.Duration, now string) error {
	processingTime := blob.Heartbeat.LastProcessingTime
	if processingTime == nil {
		processingTime = blob.Heartbeat.LastEventTime
	}
	dueAt := processingTime.Add(delay).UTC()
	if _, err := tx.ExecContext(ctx, `
		UPDATE timers SET status = 'cancelled'
		WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
		  AND operator_id = ? AND state_key = ? AND timer_kind = 'processing_time' AND status = 'pending'`,
		e.deploymentID, e.tenantID, partitionID, operator.Name, stateKey); err != nil {
		return fmt.Errorf("cancel prior heartbeat timer: %w", err)
	}
	payload, err := json.Marshal(map[string]any{
		"operator_id":       operator.Name,
		"state_key":         stateKey,
		"expected_event_id": blob.Heartbeat.LastEventID,
		"due_at":            dueAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("marshal heartbeat timer: %w", err)
	}
	timerID := heartbeatTimerID(e.deploymentID, e.tenantID, partitionID, operator.Name, stateKey, dueAt)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO timers (
			timer_id, deployment_id, tenant_id, partition_id, operator_id, state_key,
			timer_kind, due_at, payload_json, status, created_at
		) VALUES (?, ?, ?, ?, ?, ?, 'processing_time', ?, ?, 'pending', ?)
		ON CONFLICT(deployment_id, tenant_id, partition_id, operator_id, state_key, timer_kind, due_at)
		DO UPDATE SET status = 'pending', payload_json = excluded.payload_json
		WHERE timers.status != 'fired'`,
		timerID, e.deploymentID, e.tenantID, partitionID, operator.Name, stateKey,
		dueAt.Format(time.RFC3339Nano), payload, now); err != nil {
		return fmt.Errorf("insert heartbeat timer: %w", err)
	}
	return nil
}

func heartbeatTimerID(deploymentID, tenantID string, partitionID int, operatorID, stateKey string, dueAt time.Time) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("agentic-stream/timer/v1\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s",
		deploymentID, tenantID, partitionID, operatorID, stateKey, "processing_time", dueAt.Format(time.RFC3339Nano))))
	return "tmr_" + hex.EncodeToString(h[:])
}

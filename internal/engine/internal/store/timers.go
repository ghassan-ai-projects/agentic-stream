package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
)

// TimerPartitions lists the partitions that have pending processing-time timers.
func (s Store) TimerPartitions(ctx context.Context) ([]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT partition_id FROM timers
		WHERE deployment_id = ? AND tenant_id = ? AND timer_kind = 'processing_time' AND status = 'pending'
		ORDER BY partition_id`, s.deploymentID, s.tenantID)
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

// LoadDueTimers reads the partition's pending processing-time timers due at now,
// in due-time then ID order.
func (tx *Tx) LoadDueTimers(ctx context.Context, partitionID int, now time.Time) ([]domain.DueTimer, error) {
	rows, err := tx.tx.QueryContext(ctx, `
		SELECT timer_id, operator_id, state_key, due_at, payload_json FROM timers
		WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
		  AND timer_kind = 'processing_time' AND status = 'pending' AND due_at <= ?
		ORDER BY due_at, timer_id`,
		tx.deploymentID, tx.tenantID, partitionID, now.Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("query due timers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanDueTimers(rows)
}

func scanDueTimers(rows *sql.Rows) ([]domain.DueTimer, error) {
	var timers []domain.DueTimer
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
func scanDueTimer(rows *sql.Rows) (domain.DueTimer, error) {
	var timer domain.DueTimer
	var payload []byte
	if err := rows.Scan(&timer.ID, &timer.OperatorID, &timer.StateKey, &timer.DueAt, &payload); err != nil {
		return domain.DueTimer{}, fmt.Errorf("scan due timer: %w", err)
	}
	expected, err := domain.ParseTimerPayload(timer.ID, payload)
	if err != nil {
		return domain.DueTimer{}, err //nolint:wrapcheck // The domain codec names the failed timer.
	}
	timer.ExpectedEventID = expected
	return timer, nil
}

// AcknowledgeTimers marks the given pending timers fired at now.
func (tx *Tx) AcknowledgeTimers(ctx context.Context, timers []domain.DueTimer, now time.Time) error {
	placeholders := strings.TrimRight(strings.Repeat("?,", len(timers)), ",")
	args := make([]any, 0, len(timers)+2)
	args = append(args, now.Format(time.RFC3339Nano))
	for _, timer := range timers {
		args = append(args, timer.ID)
	}
	args = append(args, tx.tenantID, tx.deploymentID)
	//nolint:gosec // placeholders are generated from timer count, never user input.
	query := fmt.Sprintf(`UPDATE timers SET status = 'fired', fired_at = ?
		WHERE timer_id IN (%s) AND tenant_id = ? AND deployment_id = ? AND status = 'pending'`, placeholders)
	if _, err := tx.tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("acknowledge timers: %w", err)
	}
	return nil
}

// ArmHeartbeatTimer replaces the pending heartbeat timer of an operator state
// key: it cancels the prior one, then arms the new one, reviving a withdrawn
// timer at the same due time but never a fired one.
func (tx *Tx) ArmHeartbeatTimer(ctx context.Context, partitionID int, timer domain.HeartbeatTimer, now time.Time) error {
	if _, err := tx.tx.ExecContext(ctx, `
		UPDATE timers SET status = 'cancelled'
		WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
		  AND operator_id = ? AND state_key = ? AND timer_kind = 'processing_time' AND status = 'pending'`,
		tx.deploymentID, tx.tenantID, partitionID, timer.OperatorID, timer.StateKey); err != nil {
		return fmt.Errorf("cancel prior heartbeat timer: %w", err)
	}
	return tx.insertHeartbeatTimer(ctx, partitionID, timer, formatTime(now))
}

func (tx *Tx) insertHeartbeatTimer(ctx context.Context, partitionID int, timer domain.HeartbeatTimer, now string) error {
	if _, err := tx.tx.ExecContext(ctx, `
		INSERT INTO timers (
			timer_id, deployment_id, tenant_id, partition_id, operator_id, state_key,
			timer_kind, due_at, payload_json, status, created_at
		) VALUES (?, ?, ?, ?, ?, ?, 'processing_time', ?, ?, 'pending', ?)
		ON CONFLICT(deployment_id, tenant_id, partition_id, operator_id, state_key, timer_kind, due_at)
		DO UPDATE SET status = 'pending', payload_json = excluded.payload_json
		WHERE timers.status != 'fired'`,
		timer.ID, tx.deploymentID, tx.tenantID, partitionID, timer.OperatorID, timer.StateKey,
		timer.DueAt.Format(time.RFC3339Nano), timer.Payload, now); err != nil {
		return fmt.Errorf("insert heartbeat timer: %w", err)
	}
	return nil
}

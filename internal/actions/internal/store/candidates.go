package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

const nextCandidateSQL = `
		SELECT o.outbox_id, o.aggregate_id, c.command_json, c.command_sha256, c.status,
			c.intent_id, c.tenant_id, c.effector_route, c.normalized_target, c.idempotency_key,
			d.traceparent, d.tracestate
			, o.status, o.lease_owner, o.lease_until
		FROM outbox o
		JOIN commands c ON c.command_id = o.aggregate_id
		JOIN intents i ON i.intent_id = c.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		WHERE o.kind = 'command'
		  AND o.available_at <= ?
		  AND (o.status = 'pending' OR (o.status = 'leased' AND o.lease_until <= ?))
		ORDER BY o.outbox_id
		LIMIT 1`

const acquireLeaseSQL = `
		UPDATE outbox
		SET status = 'leased', lease_owner = ?, lease_until = ?, attempt_count = attempt_count + 1
		WHERE outbox_id = ? AND (status = 'pending' OR (status = 'leased' AND lease_until <= ?))`

// NextCandidate selects the oldest command outbox row available at now.
func (tx *Tx) NextCandidate(ctx context.Context, now time.Time) (domain.Candidate, bool, error) {
	row := tx.tx.QueryRowContext(ctx, nextCandidateSQL, formatTime(now), formatTime(now))
	var c domain.Candidate
	var trace, lease nullPair
	err := row.Scan(&c.OutboxID, &c.Command.ID, &c.Command.JSON, &c.Command.SHA, &c.CommandStatus,
		&c.Command.IntentID, &c.Command.TenantID, &c.Command.Route, &c.Command.Target, &c.Command.Idempotency,
		&trace.first, &trace.second, &c.OutboxStatus, &lease.first, &lease.second)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Candidate{}, false, nil
	}
	if err != nil {
		return domain.Candidate{}, false, fmt.Errorf("find command outbox: %w", err)
	}
	c.Trace = contractsv1.TraceContext{Traceparent: trace.first.String, Tracestate: trace.second.String}
	c.Lease = domain.Lease{Owner: lease.first.String, Until: lease.second.String, HasOwner: lease.first.Valid, HasUntil: lease.second.Valid}
	return c, true, nil
}

// nullPair holds two adjacent nullable columns scanned together.
type nullPair struct{ first, second sql.NullString }

// AcquireLease claims the outbox row for owner until the given time. It reports
// false when another dispatcher won the row.
func (tx *Tx) AcquireLease(ctx context.Context, outboxID int64, owner string, until, now time.Time) (bool, error) {
	result, err := tx.tx.ExecContext(ctx, acquireLeaseSQL, owner, formatTime(until), outboxID, formatTime(now))
	if err != nil {
		return false, fmt.Errorf("lease command outbox: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count leased command outbox rows: %w", err)
	}
	return count == 1, nil
}

// MarkCommandDispatching moves a leased command to the dispatching state.
func (tx *Tx) MarkCommandDispatching(ctx context.Context, commandID string, now time.Time) error {
	if _, err := tx.tx.ExecContext(ctx, "UPDATE commands SET status = 'dispatching', updated_at = ? WHERE command_id = ? AND status IN ('pending', 'failed', 'dispatching')", formatTime(now), commandID); err != nil {
		return fmt.Errorf("mark command dispatching: %w", err)
	}
	return nil
}

// FailInvalidCommand fails a command and its outbox row with the lease failure code.
func (tx *Tx) FailInvalidCommand(ctx context.Context, outboxID int64, commandID, code string, now time.Time) error {
	if _, err := tx.tx.ExecContext(ctx, "UPDATE commands SET status = 'failed', updated_at = ? WHERE command_id = ?", formatTime(now), commandID); err != nil {
		return fmt.Errorf("mark invalid command failed: %w", err)
	}
	if _, err := tx.tx.ExecContext(ctx, `UPDATE outbox SET status = 'failed', last_error_code = ?, lease_owner = NULL, lease_until = NULL WHERE outbox_id = ?`, code, outboxID); err != nil {
		return fmt.Errorf("mark invalid outbox failed: %w", err)
	}
	return nil
}

// CloseOutboxOnly finishes the outbox row of a command that already reached a
// terminal state.
func (tx *Tx) CloseOutboxOnly(ctx context.Context, outboxID int64, status string, now time.Time) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE outbox SET status = ?, lease_owner = NULL, lease_until = NULL, delivered_at = CASE WHEN ? = 'delivered' THEN ? ELSE delivered_at END WHERE outbox_id = ?`, status, status, formatTime(now), outboxID)
	if err != nil {
		return fmt.Errorf("finish outbox: %w", err)
	}
	return nil
}

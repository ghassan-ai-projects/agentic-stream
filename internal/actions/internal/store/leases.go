package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// LoadOutboxLease reads the lease columns of a command outbox row. It reports
// false when the row is gone.
func (tx *Tx) LoadOutboxLease(ctx context.Context, outboxID int64) (domain.OutboxLease, bool, error) {
	var lease domain.OutboxLease
	err := tx.tx.QueryRowContext(ctx, `
		SELECT status, lease_owner, lease_until FROM outbox WHERE outbox_id = ?`,
		outboxID).Scan(&lease.Status, &lease.Owner, &lease.Until)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.OutboxLease{}, false, nil
	}
	if err != nil {
		return domain.OutboxLease{}, false, fmt.Errorf("verify command lease: %w", err)
	}
	return lease, true, nil
}

// LeaseIsLive reports whether owner holds an unexpired lease on the outbox row.
func (tx *Tx) LeaseIsLive(ctx context.Context, outboxID int64, owner string, now time.Time) (bool, error) {
	_, live, err := storage.QueryOptional[int](ctx, tx.tx, `SELECT 1 FROM outbox WHERE outbox_id = ? AND status = 'leased' AND lease_owner = ? AND lease_until > ?`, outboxID, owner, sources.FormatTime(now))
	if err != nil {
		return false, fmt.Errorf("check dispatch lease: %w", err)
	}
	return live, nil
}

// RefreshLease extends a live lease. It reports false when the lease was lost.
func (tx *Tx) RefreshLease(ctx context.Context, outboxID int64, owner string, until, now time.Time) (bool, error) {
	result, err := tx.tx.ExecContext(ctx, `UPDATE outbox SET lease_until = ? WHERE outbox_id = ? AND status = 'leased' AND lease_owner = ? AND lease_until > ?`, sources.FormatTime(until), outboxID, owner, sources.FormatTime(now))
	if err != nil {
		return false, fmt.Errorf("refresh dispatch lease: %w", err)
	}
	count, err := result.RowsAffected()
	return err == nil && count == 1, nil
}

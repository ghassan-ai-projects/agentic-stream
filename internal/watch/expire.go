package watch

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"time"
)

// Expire marks due active watches inactive without deleting their audit rows.
func (e *Effector) Expire(ctx context.Context) error {
	if e == nil || e.db == nil {
		return fmt.Errorf("watch storage is required")
	}
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	return retryWhileBusy(ctx, func() error { return e.expireDue(ctx, now) })
}

// expireDue marks every active watch whose expiry has passed as expired.
func (e *Effector) expireDue(ctx context.Context, now string) error {
	if err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		if e.owner != nil && e.ownerEpoch != "" {
			if err := e.owner.Assert(ctx, tx, e.ownerEpoch); err != nil {
				return fmt.Errorf("assert watch runtime owner: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE watch_conditions SET status = 'expired', updated_at = ? WHERE status = 'active' AND expires_at <= ?", now, now); err != nil {
			return fmt.Errorf("expire watch conditions: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("expire watch transaction: %w", err)
	}
	return nil
}

// retryWhileBusy retries expire up to watchExpireAttempts times, waiting
// watchExpireBackoff between attempts, while SQLite reports writer
// contention.
func retryWhileBusy(ctx context.Context, expire func() error) error {
	var err error
	for attempt := 0; attempt < watchExpireAttempts; attempt++ {
		if err = expire(); err == nil {
			return nil
		}
		if !isSQLiteBusy(err) || attempt == watchExpireAttempts-1 {
			break
		}
		if err := awaitWatchRetry(ctx); err != nil {
			return err
		}
	}
	return fmt.Errorf("expire watch conditions: %w", err)
}

func isSQLiteBusy(err error) bool {
	return storage.IsSQLiteBusy(err)
}

func awaitWatchRetry(ctx context.Context) error {
	timer := time.NewTimer(watchExpireBackoff)
	select {
	case <-ctx.Done():
		timer.Stop()
		return fmt.Errorf("expire watch conditions: wait for retry: %w", ctx.Err())
	case <-timer.C:
	}
	return nil
}

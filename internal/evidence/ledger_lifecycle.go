package evidence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"
)

// Complete stores an exact bounded result only while the reservation remains
// owned by this runtime epoch and lease owner.
func (l *Ledger) Complete(ctx context.Context, reservation ledgerReservation, result QueryResult) error {
	if l == nil || l.DB == nil {
		return fmt.Errorf("evidence ledger is not configured")
	}
	now := time.Now().UTC()
	if l.Now != nil {
		now = l.Now().UTC()
	}
	// Persist even when the caller was canceled; keep its values (trace).
	persistenceCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := l.DB.WithTx(persistenceCtx, func(tx *sql.Tx) error {
		if err := l.assertOwner(persistenceCtx, tx); err != nil {
			return err
		}
		if err := assertAttemptRunning(persistenceCtx, tx, reservation.Key); err != nil {
			return err
		}
		return l.storeResult(persistenceCtx, tx, reservation, result, now)
	}); err != nil {
		return fmt.Errorf("complete evidence call transaction: %w", err)
	}
	return nil
}

// assertAttemptRunning requires the reserved attempt to still be the running
// episode's current, non-terminal attempt.
func assertAttemptRunning(ctx context.Context, tx *sql.Tx, key ledgerKey) error {
	var lifecycle, currentAttempt, attemptStatus string
	var currentFence int64
	if err := tx.QueryRowContext(ctx, `SELECT lifecycle_status, COALESCE(current_attempt_id, ''), current_fence FROM episodes WHERE episode_id = ?`, key.EpisodeID).Scan(&lifecycle, &currentAttempt, &currentFence); err != nil {
		return fmt.Errorf("load completion episode: %w", err)
	}
	if lifecycle != "running" || currentAttempt != key.AttemptID || currentFence != key.Fence {
		return fmt.Errorf("evidence attempt is no longer current")
	}
	if err := tx.QueryRowContext(ctx, `SELECT status FROM episode_attempts WHERE attempt_id = ? AND episode_id = ? AND fence = ?`, key.AttemptID, key.EpisodeID, key.Fence).Scan(&attemptStatus); err != nil {
		return fmt.Errorf("load completion attempt: %w", err)
	}
	if attemptStatus != "dispatched" && attemptStatus != "running" {
		return fmt.Errorf("evidence attempt is no longer active")
	}
	return nil
}

// storeResult completes the reservation with its exact result, only while
// this lease owner and runtime epoch still hold an unexpired lease.
func (l *Ledger) storeResult(ctx context.Context, tx *sql.Tx, reservation ledgerReservation, result QueryResult, now time.Time) error {
	hash := sha256.Sum256(result.JSON)
	key := reservation.Key
	res, err := tx.ExecContext(ctx, `UPDATE evidence_call_ledger SET status = 'completed', result_json = ?, result_sha256 = ?, result_bytes = ?, row_count = ?, completed_at = ? WHERE tenant_id = ? AND episode_id = ? AND attempt_id = ? AND fence = ? AND call_id = ? AND status = 'running' AND request_sha256 = ? AND token_id = ? AND lease_owner = ? AND runtime_epoch = ? AND lease_until > ?`, result.JSON, hash[:], len(result.JSON), result.RowCount, formatLedgerTime(now), key.TenantID, key.EpisodeID, key.AttemptID, key.Fence, key.CallID, reservation.RequestSHA256, reservation.TokenID, l.LeaseOwner, reservation.RuntimeEpoch, formatLedgerTime(now))
	if err != nil {
		return fmt.Errorf("complete evidence call: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("evidence call reservation is no longer owned")
	}
	return nil
}

// Fail records a stable terminal failure for a reserved call.
func (l *Ledger) Fail(ctx context.Context, reservation ledgerReservation, code string) error {
	if l == nil || l.DB == nil {
		return fmt.Errorf("evidence ledger is not configured")
	}
	// Persist even when the caller was canceled; keep its values (trace).
	persistenceCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	if l.Now != nil {
		now = l.Now().UTC()
	}
	if err := l.DB.WithTx(persistenceCtx, func(tx *sql.Tx) error {
		if err := l.assertOwner(persistenceCtx, tx); err != nil {
			return err
		}
		result, err := tx.ExecContext(persistenceCtx, `UPDATE evidence_call_ledger SET status = 'failed', error_code = ?, completed_at = ? WHERE tenant_id = ? AND episode_id = ? AND attempt_id = ? AND fence = ? AND call_id = ? AND status = 'running' AND request_sha256 = ? AND token_id = ? AND lease_owner = ? AND runtime_epoch = ? AND lease_until > ?`, code, formatLedgerTime(now), reservation.Key.TenantID, reservation.Key.EpisodeID, reservation.Key.AttemptID, reservation.Key.Fence, reservation.Key.CallID, reservation.RequestSHA256, reservation.TokenID, l.LeaseOwner, reservation.RuntimeEpoch, formatLedgerTime(now))
		if err != nil {
			return fmt.Errorf("fail evidence call: %w", err)
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return fmt.Errorf("evidence call reservation is no longer owned")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("fail evidence call transaction: %w", err)
	}
	return nil
}

// ReclaimExpired marks abandoned reservations terminal so a crash cannot
// leave the same call permanently in-flight. Retrying requires a new call ID.
func (l *Ledger) ReclaimExpired(ctx context.Context, now time.Time) error {
	if l == nil || l.DB == nil || l.RuntimeEpoch == "" {
		return fmt.Errorf("evidence ledger is not configured")
	}
	err := l.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := l.assertOwner(ctx, tx); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE evidence_call_ledger SET status = 'interrupted', error_code = 'lease_expired', completed_at = ? WHERE status = 'running' AND (lease_until <= ? OR runtime_epoch <> ?)`, formatLedgerTime(now.UTC()), formatLedgerTime(now.UTC()), l.RuntimeEpoch)
		if err != nil {
			return fmt.Errorf("reclaim evidence calls: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reclaim evidence call ledger: %w", err)
	}
	return nil
}

// Recover marks unfinished reservations from prior owners interrupted before
// the runtime becomes ready. A later execution must use a new fenced attempt.
func (l *Ledger) Recover(ctx context.Context) error {
	if l == nil || l.RuntimeEpoch == "" {
		return fmt.Errorf("evidence ledger runtime epoch is not configured")
	}
	now := time.Now().UTC()
	if l.Now != nil {
		now = l.Now().UTC()
	}
	return l.ReclaimExpired(ctx, now)
}

// RecoverTx interrupts unfinished reservations from prior runtime epochs in
// the caller's transaction. It does not persist capability bytes or reuse a
// call identity.
func (l *Ledger) RecoverTx(ctx context.Context, tx *sql.Tx, now time.Time) (int, error) {
	if l == nil || tx == nil || l.RuntimeEpoch == "" {
		return 0, fmt.Errorf("evidence ledger recovery is not configured")
	}
	if err := l.assertOwner(ctx, tx); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE evidence_call_ledger
		SET status = 'interrupted', error_code = 'runtime_restart', completed_at = ?
		WHERE status = 'running' AND runtime_epoch <> ?`,
		formatLedgerTime(now.UTC()), l.RuntimeEpoch)
	if err != nil {
		return 0, fmt.Errorf("recover evidence calls: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count recovered evidence calls: %w", err)
	}
	return int(count), nil
}

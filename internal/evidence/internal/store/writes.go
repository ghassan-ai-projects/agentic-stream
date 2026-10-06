package store

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
)

// InsertReservation persists a new running call.
func (tx *Tx) InsertReservation(ctx context.Context, call domain.Call, pending domain.Reservation, now time.Time, lease time.Duration, leaseOwner string) error {
	key := pending.Key
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO evidence_call_ledger (tenant_id, episode_id, attempt_id, fence, call_id, token_id, runtime_epoch, tool_name, situation_id, situation_version, entity_id, time_from, time_until, max_rows, max_bytes, deadline, request_sha256, status, lease_owner, lease_until, reserved_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'running', ?, ?, ?)`, key.TenantID, key.EpisodeID, key.AttemptID, key.Fence, key.CallID, pending.TokenID, pending.RuntimeEpoch, call.ToolName, call.SituationID, call.SituationVersion, call.EntityID, formatLedgerTime(call.From), formatLedgerTime(call.Until), call.MaxRows, call.MaxBytes, formatLedgerTime(call.Deadline), pending.RequestSHA256, leaseOwner, formatLedgerTime(now.Add(lease)), formatLedgerTime(now))
	if err != nil {
		return fmt.Errorf("reserve evidence call: %w", err)
	}
	return nil
}

// StoreResult persists exact bytes under the unchanged lease predicate.
func (tx *Tx) StoreResult(ctx context.Context, reservation domain.Reservation, result domain.QueryResult, now time.Time, leaseOwner string) error {
	hash := sha256.Sum256(result.JSON)
	key := reservation.Key
	res, err := tx.tx.ExecContext(ctx, `UPDATE evidence_call_ledger SET status = 'completed', result_json = ?, result_sha256 = ?, result_bytes = ?, row_count = ?, completed_at = ? WHERE tenant_id = ? AND episode_id = ? AND attempt_id = ? AND fence = ? AND call_id = ? AND status = 'running' AND request_sha256 = ? AND token_id = ? AND lease_owner = ? AND runtime_epoch = ? AND lease_until > ?`, result.JSON, hash[:], len(result.JSON), result.RowCount, formatLedgerTime(now), key.TenantID, key.EpisodeID, key.AttemptID, key.Fence, key.CallID, reservation.RequestSHA256, reservation.TokenID, leaseOwner, reservation.RuntimeEpoch, formatLedgerTime(now))
	if err != nil {
		return fmt.Errorf("complete evidence call: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("evidence call reservation is no longer owned")
	}
	return nil
}

// Fail records a stable terminal failure under the lease predicate.
func (tx *Tx) Fail(ctx context.Context, reservation domain.Reservation, code string, now time.Time, leaseOwner string) error {
	result, err := tx.tx.ExecContext(ctx, `UPDATE evidence_call_ledger SET status = 'failed', error_code = ?, completed_at = ? WHERE tenant_id = ? AND episode_id = ? AND attempt_id = ? AND fence = ? AND call_id = ? AND status = 'running' AND request_sha256 = ? AND token_id = ? AND lease_owner = ? AND runtime_epoch = ? AND lease_until > ?`, code, formatLedgerTime(now), reservation.Key.TenantID, reservation.Key.EpisodeID, reservation.Key.AttemptID, reservation.Key.Fence, reservation.Key.CallID, reservation.RequestSHA256, reservation.TokenID, leaseOwner, reservation.RuntimeEpoch, formatLedgerTime(now))
	if err != nil {
		return fmt.Errorf("fail evidence call: %w", err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return fmt.Errorf("evidence call reservation is no longer owned")
	}
	return nil
}

// ReclaimExpired interrupts abandoned reservations.
func (tx *Tx) ReclaimExpired(ctx context.Context, now time.Time) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE evidence_call_ledger SET status = 'interrupted', error_code = 'lease_expired', completed_at = ? WHERE status = 'running' AND (lease_until <= ? OR runtime_epoch <> ?)`, formatLedgerTime(now.UTC()), formatLedgerTime(now.UTC()), tx.epoch)
	if err != nil {
		return fmt.Errorf("reclaim evidence calls: %w", err)
	}
	return nil
}

// Recover interrupts prior-epoch reservations in the joined transaction.
func (tx *Tx) Recover(ctx context.Context, now time.Time) (int, error) {
	result, err := tx.tx.ExecContext(ctx, recoverEvidenceCallsSQL, formatLedgerTime(now.UTC()), tx.epoch)
	if err != nil {
		return 0, fmt.Errorf("recover evidence calls: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count recovered evidence calls: %w", err)
	}
	return int(count), nil
}

const recoverEvidenceCallsSQL = `
 UPDATE evidence_call_ledger
 SET status = 'interrupted', error_code = 'runtime_restart', completed_at = ?
 WHERE status = 'running' AND runtime_epoch <> ?`

func formatLedgerTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

package evidence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Ledger stores evidence-call reservations and completed bounded results.
// Capability bytes are never persisted.
type Ledger struct {
	DB           *storage.DB
	Owner        *storage.RuntimeOwner
	LeaseOwner   string
	RuntimeEpoch string
	Lease        time.Duration
	Now          func() time.Time
}

type ledgerKey struct {
	TenantID  string
	EpisodeID string
	AttemptID string
	Fence     int64
	CallID    string
}

type ledgerReservation struct {
	Key           ledgerKey
	RequestSHA256 []byte
	TokenID       string
	RuntimeEpoch  string
	Completed     *QueryResult
	ResultSHA256  []byte
	Status        string
	Created       bool
}

// Reserve atomically validates the current live attempt and reserves a call.
// Completed calls return their exact stored result without invoking a provider.
func (l *Ledger) Reserve(ctx context.Context, call Call, tokenID, runtimeEpoch string) (ledgerReservation, error) {
	if l == nil || l.DB == nil || l.LeaseOwner == "" || l.RuntimeEpoch == "" || runtimeEpoch == "" || runtimeEpoch != l.RuntimeEpoch || tokenID == "" {
		return ledgerReservation{}, fmt.Errorf("evidence ledger is not configured")
	}
	now := time.Now().UTC()
	if l.Now != nil {
		now = l.Now().UTC()
	}
	lease := l.Lease
	if lease <= 0 {
		lease = time.Minute
	}
	fingerprint, err := callFingerprint(call)
	if err != nil {
		return ledgerReservation{}, err
	}
	key := ledgerKey{TenantID: call.TenantID, EpisodeID: call.EpisodeID, AttemptID: call.AttemptID, Fence: call.Fence, CallID: call.CallID}
	var reservation ledgerReservation
	err = l.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := l.assertOwner(ctx, tx); err != nil {
			return err
		}
		var lifecycle, currentAttempt, attemptStatus string
		var currentFence int64
		if err := tx.QueryRowContext(ctx, `SELECT lifecycle_status, COALESCE(current_attempt_id, ''), current_fence FROM episodes WHERE episode_id = ? AND tenant_id = ?`, call.EpisodeID, call.TenantID).Scan(&lifecycle, &currentAttempt, &currentFence); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("evidence episode is unknown")
			}
			return fmt.Errorf("load evidence episode: %w", err)
		}
		if currentAttempt != call.AttemptID || currentFence != call.Fence || lifecycle == "concluded" || lifecycle == "closed" || lifecycle == "superseded" || lifecycle == "expired" || lifecycle == "abandoned" {
			return fmt.Errorf("evidence attempt is stale")
		}
		if err := tx.QueryRowContext(ctx, `SELECT status FROM episode_attempts WHERE episode_id = ? AND attempt_id = ? AND fence = ?`, call.EpisodeID, call.AttemptID, call.Fence).Scan(&attemptStatus); err != nil {
			return fmt.Errorf("load evidence attempt: %w", err)
		}
		if attemptStatus != "dispatched" && attemptStatus != "running" {
			return fmt.Errorf("evidence attempt is terminal")
		}
		var existingHash, resultJSON, resultHash []byte
		var existingTokenID, existingEpoch, status string
		var rowCount, resultBytes sql.NullInt64
		err := tx.QueryRowContext(ctx, `SELECT request_sha256, token_id, runtime_epoch, status, result_json, result_sha256, row_count, result_bytes FROM evidence_call_ledger WHERE tenant_id = ? AND episode_id = ? AND attempt_id = ? AND fence = ? AND call_id = ?`, key.TenantID, key.EpisodeID, key.AttemptID, key.Fence, key.CallID).Scan(&existingHash, &existingTokenID, &existingEpoch, &status, &resultJSON, &resultHash, &rowCount, &resultBytes)
		if err == nil {
			if string(existingHash) != string(fingerprint) {
				return fmt.Errorf("evidence call identity was reused with different request")
			}
			if existingTokenID != tokenID || existingEpoch != runtimeEpoch {
				return fmt.Errorf("evidence call token identity mismatch")
			}
			reservation = ledgerReservation{Key: key, RequestSHA256: fingerprint, TokenID: tokenID, RuntimeEpoch: runtimeEpoch, Status: status, ResultSHA256: resultHash}
			if status == "completed" {
				if resultBytes.Int64 < 0 || uint64(resultBytes.Int64) != uint64(len(resultJSON)) || len(resultHash) != sha256.Size || rowCount.Int64 < 0 {
					return fmt.Errorf("stored evidence result is malformed")
				}
				hash := sha256.Sum256(resultJSON)
				if string(hash[:]) != string(resultHash) {
					return fmt.Errorf("stored evidence result digest mismatch")
				}
				reservation.Completed = &QueryResult{JSON: append([]byte(nil), resultJSON...), RowCount: uint64(rowCount.Int64)}
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load evidence call: %w", err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO evidence_call_ledger (tenant_id, episode_id, attempt_id, fence, call_id, token_id, runtime_epoch, tool_name, situation_id, situation_version, entity_id, time_from, time_until, max_rows, max_bytes, deadline, request_sha256, status, lease_owner, lease_until, reserved_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'running', ?, ?, ?)`, key.TenantID, key.EpisodeID, key.AttemptID, key.Fence, key.CallID, tokenID, runtimeEpoch, call.ToolName, call.SituationID, call.SituationVersion, call.EntityID, formatLedgerTime(call.From), formatLedgerTime(call.Until), call.MaxRows, call.MaxBytes, formatLedgerTime(call.Deadline), fingerprint, l.LeaseOwner, formatLedgerTime(now.Add(lease)), formatLedgerTime(now))
		if err != nil {
			return fmt.Errorf("reserve evidence call: %w", err)
		}
		reservation = ledgerReservation{Key: key, RequestSHA256: fingerprint, TokenID: tokenID, RuntimeEpoch: runtimeEpoch, Status: "running", Created: true}
		return nil
	})
	if err != nil {
		return ledgerReservation{}, fmt.Errorf("reserve evidence call transaction: %w", err)
	}
	return reservation, nil
}

// Complete stores an exact bounded result only while the reservation remains
// owned by this runtime epoch and lease owner.
func (l *Ledger) Complete(ctx context.Context, reservation ledgerReservation, result QueryResult) error {
	if l == nil || l.DB == nil {
		return fmt.Errorf("evidence ledger is not configured")
	}
	hash := sha256.Sum256(result.JSON)
	now := time.Now().UTC()
	if l.Now != nil {
		now = l.Now().UTC()
	}
	persistenceCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := l.DB.WithTx(persistenceCtx, func(tx *sql.Tx) error {
		if err := l.assertOwner(persistenceCtx, tx); err != nil {
			return err
		}
		var lifecycle, currentAttempt, attemptStatus string
		var currentFence int64
		if err := tx.QueryRowContext(persistenceCtx, `SELECT lifecycle_status, COALESCE(current_attempt_id, ''), current_fence FROM episodes WHERE episode_id = ?`, reservation.Key.EpisodeID).Scan(&lifecycle, &currentAttempt, &currentFence); err != nil {
			return fmt.Errorf("load completion episode: %w", err)
		}
		if lifecycle != "running" || currentAttempt != reservation.Key.AttemptID || currentFence != reservation.Key.Fence {
			return fmt.Errorf("evidence attempt is no longer current")
		}
		if err := tx.QueryRowContext(persistenceCtx, `SELECT status FROM episode_attempts WHERE attempt_id = ? AND episode_id = ? AND fence = ?`, reservation.Key.AttemptID, reservation.Key.EpisodeID, reservation.Key.Fence).Scan(&attemptStatus); err != nil {
			return fmt.Errorf("load completion attempt: %w", err)
		}
		if attemptStatus != "dispatched" && attemptStatus != "running" {
			return fmt.Errorf("evidence attempt is no longer active")
		}
		res, err := tx.ExecContext(persistenceCtx, `UPDATE evidence_call_ledger SET status = 'completed', result_json = ?, result_sha256 = ?, result_bytes = ?, row_count = ?, completed_at = ? WHERE tenant_id = ? AND episode_id = ? AND attempt_id = ? AND fence = ? AND call_id = ? AND status = 'running' AND request_sha256 = ? AND token_id = ? AND lease_owner = ? AND runtime_epoch = ? AND lease_until > ?`, result.JSON, hash[:], len(result.JSON), result.RowCount, formatLedgerTime(now), reservation.Key.TenantID, reservation.Key.EpisodeID, reservation.Key.AttemptID, reservation.Key.Fence, reservation.Key.CallID, reservation.RequestSHA256, reservation.TokenID, l.LeaseOwner, reservation.RuntimeEpoch, formatLedgerTime(now))
		if err != nil {
			return fmt.Errorf("complete evidence call: %w", err)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return fmt.Errorf("evidence call reservation is no longer owned")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("complete evidence call transaction: %w", err)
	}
	return nil
}

// Fail records a stable terminal failure for a reserved call.
func (l *Ledger) Fail(ctx context.Context, reservation ledgerReservation, code string) error {
	if l == nil || l.DB == nil {
		return fmt.Errorf("evidence ledger is not configured")
	}
	persistenceCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
		return err
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

func (l *Ledger) assertOwner(ctx context.Context, tx *sql.Tx) error {
	if l.Owner == nil {
		return nil
	}
	return l.Owner.Assert(ctx, tx, l.RuntimeEpoch)
}

func callFingerprint(call Call) ([]byte, error) {
	document := struct {
		EpisodeID        string               `json:"episode_id"`
		CallID           string               `json:"call_id"`
		ToolName         string               `json:"tool_name"`
		TenantID         string               `json:"tenant_id"`
		SituationID      string               `json:"situation_id"`
		EntityID         string               `json:"entity_id"`
		AttemptID        string               `json:"attempt_id"`
		SituationVersion int64                `json:"situation_version"`
		Fence            int64                `json:"fence"`
		Arguments        EvidenceGetArguments `json:"arguments"`
		Deadline         string               `json:"deadline"`
		From             string               `json:"from"`
		Until            string               `json:"until"`
		MaxRows          uint64               `json:"max_rows"`
		MaxBytes         uint64               `json:"max_bytes"`
		Traceparent      string               `json:"traceparent"`
		Tracestate       string               `json:"tracestate"`
	}{call.EpisodeID, call.CallID, call.ToolName, call.TenantID, call.SituationID, call.EntityID, call.AttemptID, call.SituationVersion, call.Fence, call.Arguments, formatLedgerTime(call.Deadline), formatLedgerTime(call.From), formatLedgerTime(call.Until), call.MaxRows, call.MaxBytes, call.Trace.Traceparent, call.Trace.Tracestate}
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("marshal evidence fingerprint: %w", err)
	}
	hash := sha256.Sum256(raw)
	return hash[:], nil
}

func formatLedgerTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

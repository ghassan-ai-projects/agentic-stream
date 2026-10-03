package evidence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Ledger stores evidence-call reservations and completed bounded results.
// Capability bytes are never persisted.
type Ledger struct {
	DB           *storage.DB
	Owner        *runtimecontrol.RuntimeOwner
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
		if err := assertLiveAttempt(ctx, tx, call); err != nil {
			return err
		}
		existing, err := loadReservation(ctx, tx, key, fingerprint, tokenID, runtimeEpoch)
		if err != nil || existing != nil {
			reservation = derefReservation(existing)
			return err
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

// assertLiveAttempt requires the call's attempt to be the episode's current,
// non-terminal attempt.
func assertLiveAttempt(ctx context.Context, tx *sql.Tx, call Call) error {
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
	return nil
}

// loadReservation returns the existing reservation for key, or nil when the
// call is new. A reused call identity must carry the same request and token,
// and a completed call returns its verified stored result.
func loadReservation(ctx context.Context, tx *sql.Tx, key ledgerKey, fingerprint []byte, tokenID, runtimeEpoch string) (*ledgerReservation, error) {
	var existingHash, resultJSON, resultHash []byte
	var existingTokenID, existingEpoch, status string
	var rowCount, resultBytes sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT request_sha256, token_id, runtime_epoch, status, result_json, result_sha256, row_count, result_bytes FROM evidence_call_ledger WHERE tenant_id = ? AND episode_id = ? AND attempt_id = ? AND fence = ? AND call_id = ?`, key.TenantID, key.EpisodeID, key.AttemptID, key.Fence, key.CallID).Scan(&existingHash, &existingTokenID, &existingEpoch, &status, &resultJSON, &resultHash, &rowCount, &resultBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load evidence call: %w", err)
	}
	if string(existingHash) != string(fingerprint) {
		return nil, fmt.Errorf("evidence call identity was reused with different request")
	}
	if existingTokenID != tokenID || existingEpoch != runtimeEpoch {
		return nil, fmt.Errorf("evidence call token identity mismatch")
	}
	reservation := &ledgerReservation{Key: key, RequestSHA256: fingerprint, TokenID: tokenID, RuntimeEpoch: runtimeEpoch, Status: status, ResultSHA256: resultHash}
	if status != "completed" {
		return reservation, nil
	}
	if resultBytes.Int64 < 0 || uint64(resultBytes.Int64) != uint64(len(resultJSON)) || len(resultHash) != sha256.Size || rowCount.Int64 < 0 {
		return nil, fmt.Errorf("stored evidence result is malformed")
	}
	hash := sha256.Sum256(resultJSON)
	if string(hash[:]) != string(resultHash) {
		return nil, fmt.Errorf("stored evidence result digest mismatch")
	}
	reservation.Completed = &QueryResult{JSON: append([]byte(nil), resultJSON...), RowCount: uint64(rowCount.Int64)}
	return reservation, nil
}

func derefReservation(reservation *ledgerReservation) ledgerReservation {
	if reservation == nil {
		return ledgerReservation{}
	}
	return *reservation
}

func (l *Ledger) assertOwner(ctx context.Context, tx *sql.Tx) error {
	if l.Owner == nil {
		return nil
	}
	if err := l.Owner.Assert(ctx, tx, l.RuntimeEpoch); err != nil {
		return fmt.Errorf("evidence ledger runtime ownership lost: %w", err)
	}
	return nil
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

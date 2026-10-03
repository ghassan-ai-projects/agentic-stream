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
	now, lease := l.now(), l.reservationLease()
	fingerprint, err := callFingerprint(call)
	if err != nil {
		return ledgerReservation{}, err
	}
	key := ledgerKey{TenantID: call.TenantID, EpisodeID: call.EpisodeID, AttemptID: call.AttemptID, Fence: call.Fence, CallID: call.CallID}
	pending := ledgerReservation{Key: key, RequestSHA256: fingerprint, TokenID: tokenID, RuntimeEpoch: runtimeEpoch, Status: "running", Created: true}
	return l.reserveCall(ctx, call, pending, now, lease)
}

func (l *Ledger) now() time.Time {
	now := time.Now().UTC()
	if l.Now != nil {
		now = l.Now().UTC()
	}
	return now
}

func (l *Ledger) reservationLease() time.Duration {
	if l.Lease <= 0 {
		return time.Minute
	}
	return l.Lease
}

func (l *Ledger) reserveCall(ctx context.Context, call Call, pending ledgerReservation, now time.Time, lease time.Duration) (ledgerReservation, error) {
	var reservation ledgerReservation
	err := l.DB.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		reservation, err = l.reserveTx(ctx, tx, call, pending, now, lease)
		return err
	})
	if err != nil {
		return ledgerReservation{}, fmt.Errorf("reserve evidence call transaction: %w", err)
	}
	return reservation, nil
}

func (l *Ledger) reserveTx(ctx context.Context, tx *sql.Tx, call Call, pending ledgerReservation, now time.Time, lease time.Duration) (ledgerReservation, error) {
	if err := l.assertOwner(ctx, tx); err != nil {
		return ledgerReservation{}, err
	}
	if err := assertLiveAttempt(ctx, tx, call); err != nil {
		return ledgerReservation{}, err
	}
	existing, err := loadReservation(ctx, tx, pending.Key, pending.RequestSHA256, pending.TokenID, pending.RuntimeEpoch)
	if err != nil || existing != nil {
		return derefReservation(existing), err
	}
	if err := l.insertReservation(ctx, tx, call, pending, now, lease); err != nil {
		return ledgerReservation{}, err
	}
	return pending, nil
}

func (l *Ledger) insertReservation(ctx context.Context, tx *sql.Tx, call Call, pending ledgerReservation, now time.Time, lease time.Duration) error {
	key := pending.Key
	_, err := tx.ExecContext(ctx, `INSERT INTO evidence_call_ledger (tenant_id, episode_id, attempt_id, fence, call_id, token_id, runtime_epoch, tool_name, situation_id, situation_version, entity_id, time_from, time_until, max_rows, max_bytes, deadline, request_sha256, status, lease_owner, lease_until, reserved_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'running', ?, ?, ?)`, key.TenantID, key.EpisodeID, key.AttemptID, key.Fence, key.CallID, pending.TokenID, pending.RuntimeEpoch, call.ToolName, call.SituationID, call.SituationVersion, call.EntityID, formatLedgerTime(call.From), formatLedgerTime(call.Until), call.MaxRows, call.MaxBytes, formatLedgerTime(call.Deadline), pending.RequestSHA256, l.LeaseOwner, formatLedgerTime(now.Add(lease)), formatLedgerTime(now))
	if err != nil {
		return fmt.Errorf("reserve evidence call: %w", err)
	}
	return nil
}

// assertLiveAttempt requires the call's attempt to be the episode's current,
// non-terminal attempt.
func assertLiveAttempt(ctx context.Context, tx *sql.Tx, call Call) error {
	var lifecycle, currentAttempt string
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
	return assertLiveAttemptStatus(ctx, tx, call)
}

func assertLiveAttemptStatus(ctx context.Context, tx *sql.Tx, call Call) error {
	var attemptStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM episode_attempts WHERE episode_id = ? AND attempt_id = ? AND fence = ?`, call.EpisodeID, call.AttemptID, call.Fence).Scan(&attemptStatus); err != nil {
		return fmt.Errorf("load evidence attempt: %w", err)
	}
	if attemptStatus != "dispatched" && attemptStatus != "running" {
		return fmt.Errorf("evidence attempt is terminal")
	}
	return nil
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
	document := fingerprintDocument{call.EpisodeID, call.CallID, call.ToolName, call.TenantID, call.SituationID, call.EntityID, call.AttemptID, call.SituationVersion, call.Fence, call.Arguments, formatLedgerTime(call.Deadline), formatLedgerTime(call.From), formatLedgerTime(call.Until), call.MaxRows, call.MaxBytes, call.Trace.Traceparent, call.Trace.Tracestate}
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("marshal evidence fingerprint: %w", err)
	}
	hash := sha256.Sum256(raw)
	return hash[:], nil
}

func formatLedgerTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

// fingerprintDocument preserves the encoded request identity and field order.
type fingerprintDocument struct {
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
}

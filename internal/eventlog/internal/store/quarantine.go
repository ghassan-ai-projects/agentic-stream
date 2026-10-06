package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
)

// QuarantineDigest loads the payload digest already quarantined under an
// event id, if any.
func (u *Unit) QuarantineDigest(ctx context.Context, tenantID, eventID string) ([]byte, error) {
	var existingDigest []byte
	err := u.tx.QueryRowContext(ctx, "SELECT payload_sha256 FROM event_quarantine WHERE tenant_id = ? AND event_id = ?", tenantID, eventID).Scan(&existingDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load existing quarantine: %w", err)
	}
	return existingDigest, nil
}

// UpsertQuarantine inserts the record or counts a repeated delivery of the
// same payload, returning the number of affected rows.
func (u *Unit) UpsertQuarantine(ctx context.Context, payload domain.QuarantinePayload, tenantID, reason, now string) (int64, error) {
	result, err := u.tx.ExecContext(ctx, upsertQuarantineSQL,
		payload.QuarantineID, tenantID, payload.EventID, payload.EventType, payload.SchemaVersion, payload.Source,
		reason, payload.Payload, payload.Digest, now, now)
	if err != nil {
		return 0, fmt.Errorf("persist event quarantine: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count quarantine update: %w", err)
	}
	return count, nil
}

const upsertQuarantineSQL = `
		INSERT INTO event_quarantine (
			quarantine_id, tenant_id, event_id, event_type, schema_version, source,
			reason_code, payload_json, payload_sha256, status, first_seen_at, last_seen_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'quarantined', ?, ?)
		ON CONFLICT(tenant_id, event_id) DO UPDATE SET
			attempt_count = MIN(attempt_count + 1, 10), last_seen_at = excluded.last_seen_at,
			reason_code = excluded.reason_code, payload_json = excluded.payload_json,
		payload_sha256 = excluded.payload_sha256,
		status = CASE WHEN attempt_count >= 10 THEN 'rejected' ELSE status END
		WHERE event_quarantine.payload_sha256 = excluded.payload_sha256`

// RejectQuarantineConflict marks a conflicting record rejected.
func (u *Unit) RejectQuarantineConflict(ctx context.Context, tenantID, eventID, now string) error {
	if _, err := u.tx.ExecContext(ctx, "UPDATE event_quarantine SET status = 'rejected', reason_code = 'event_id_hash_conflict', last_seen_at = ? WHERE tenant_id = ? AND event_id = ?", now, tenantID, eventID); err != nil {
		return fmt.Errorf("record quarantine hash conflict: %w", err)
	}
	return nil
}

// QuarantineStatus reads one record's lifecycle status.
func (u *Unit) QuarantineStatus(ctx context.Context, tenantID, eventID string) (string, error) {
	var status string
	if err := u.tx.QueryRowContext(ctx, "SELECT status FROM event_quarantine WHERE tenant_id = ? AND event_id = ?", tenantID, eventID).Scan(&status); err != nil {
		return "", fmt.Errorf("read quarantine status: %w", err)
	}
	return status, nil
}

// InsertOverflowGap records the one gap a spent quarantine record owes.
func (u *Unit) InsertOverflowGap(ctx context.Context, gapID, tenantID, now string) error {
	if _, err := u.tx.ExecContext(ctx, `INSERT INTO event_gaps (gap_id, tenant_id, partition_id, from_position, to_position, reason_code, created_at) VALUES (?, ?, 0, 0, 0, 'quarantine_retry_exhausted', ?) ON CONFLICT(gap_id) DO NOTHING`, gapID, tenantID, now); err != nil {
		return fmt.Errorf("record quarantine overflow gap: %w", err)
	}
	return nil
}

// ReleaseQuarantined marks one quarantined record released; it fails when no
// quarantined record was available.
func (u *Unit) ReleaseQuarantined(ctx context.Context, tenantID, eventID, now string) error {
	result, err := u.tx.ExecContext(ctx, `UPDATE event_quarantine SET status = 'released', released_at = ?, last_seen_at = ? WHERE tenant_id = ? AND event_id = ? AND status = 'quarantined'`, now, now, tenantID, eventID)
	if err != nil {
		return fmt.Errorf("release quarantined event: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count released event: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("quarantined event is not available for release")
	}
	return nil
}

// ReleasedEnvelope returns a released, not-yet-redriven record's envelope.
func (u *Unit) ReleasedEnvelope(ctx context.Context, tenantID, eventID string) (contractsv1.Envelope, error) {
	var payload []byte
	var status string
	var redrivenAt sql.NullString
	if err := u.tx.QueryRowContext(ctx, "SELECT payload_json, status, redriven_at FROM event_quarantine WHERE tenant_id = ? AND event_id = ?", tenantID, eventID).Scan(&payload, &status, &redrivenAt); err != nil {
		return contractsv1.Envelope{}, fmt.Errorf("load released quarantine: %w", err)
	}
	if status != "released" || redrivenAt.Valid {
		return contractsv1.Envelope{}, fmt.Errorf("quarantine %s is not released", eventID)
	}
	var env contractsv1.Envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return contractsv1.Envelope{}, fmt.Errorf("decode released envelope: %w", err)
	}
	return env, nil
}

// MarkRedriven marks a released record redriven.
func (u *Unit) MarkRedriven(ctx context.Context, tenantID, eventID, now string) error {
	if _, err := u.tx.ExecContext(ctx, "UPDATE event_quarantine SET redriven_at = ? WHERE tenant_id = ? AND event_id = ? AND status = 'released'", now, tenantID, eventID); err != nil {
		return fmt.Errorf("mark quarantine redriven: %w", err)
	}
	return nil
}

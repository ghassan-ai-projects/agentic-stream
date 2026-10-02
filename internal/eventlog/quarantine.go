package eventlog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// Quarantine records an invalid event durably without placing it in the
// executable event log. Repeated delivery increments a bounded retry count.
func (l *EventLog) Quarantine(ctx context.Context, tenantID string, env map[string]any, reason, now string) error {
	if tenantID == "" || reason == "" || now == "" {
		return fmt.Errorf("tenant, reason, and time are required")
	}
	eventID, _ := env["id"].(string)
	eventType, _ := env["type"].(string)
	schemaVersion, _ := env["schema_version"].(string)
	source, _ := env["source"].(string)
	payload, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal quarantined payload: %w", err)
	}
	if eventID == "" {
		rawDigest := sha256.Sum256(payload)
		eventID = "payload:" + hex.EncodeToString(rawDigest[:12])
	}
	digest := sha256.Sum256(payload)
	idDigest := sha256.Sum256(append([]byte(eventID+"|"), payload...))
	quarantineID := "q_" + hex.EncodeToString(idDigest[:12])
	record := quarantineRecord{
		quarantineID: quarantineID, tenantID: tenantID, eventID: eventID, eventType: eventType,
		schemaVersion: schemaVersion, source: source, reason: reason, payload: payload, digest: digest[:], now: now,
	}
	conflict := false
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		conflict, err = record.persist(ctx, tx)
		return err
	}); err != nil {
		return fmt.Errorf("quarantine event transaction: %w", err)
	}
	if conflict {
		return fmt.Errorf("event id %s has conflicting quarantined payload", eventID)
	}
	return nil
}

// quarantineRecord is one invalid event as it is quarantined.
type quarantineRecord struct {
	quarantineID, tenantID, eventID, eventType, schemaVersion, source, reason string
	payload, digest                                                           []byte
	now                                                                       string
}

// persist inserts the record or counts a repeated delivery of the same
// payload. A different payload under the same event ID is a conflict: the
// existing record is rejected and persist reports true. A record whose retry
// count is exhausted also records an event gap.
func (q quarantineRecord) persist(ctx context.Context, tx *sql.Tx) (bool, error) {
	var existingDigest []byte
	existingErr := tx.QueryRowContext(ctx, "SELECT payload_sha256 FROM event_quarantine WHERE tenant_id = ? AND event_id = ?", q.tenantID, q.eventID).Scan(&existingDigest)
	if existingErr == nil && !bytes.Equal(existingDigest, q.digest) {
		return true, q.rejectConflict(ctx, tx)
	}
	if existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows) {
		return false, fmt.Errorf("load existing quarantine: %w", existingErr)
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO event_quarantine (
			quarantine_id, tenant_id, event_id, event_type, schema_version, source,
			reason_code, payload_json, payload_sha256, status, first_seen_at, last_seen_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'quarantined', ?, ?)
		ON CONFLICT(tenant_id, event_id) DO UPDATE SET
			attempt_count = MIN(attempt_count + 1, 10), last_seen_at = excluded.last_seen_at,
			reason_code = excluded.reason_code, payload_json = excluded.payload_json,
		payload_sha256 = excluded.payload_sha256,
		status = CASE WHEN attempt_count >= 10 THEN 'rejected' ELSE status END
		WHERE event_quarantine.payload_sha256 = excluded.payload_sha256`,
		q.quarantineID, q.tenantID, q.eventID, q.eventType, q.schemaVersion, q.source,
		q.reason, q.payload, q.digest, q.now, q.now)
	if err != nil {
		return false, fmt.Errorf("persist event quarantine: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count quarantine update: %w", err)
	}
	if count == 0 {
		return true, q.rejectConflict(ctx, tx)
	}
	return false, q.recordOverflow(ctx, tx)
}

func (q quarantineRecord) rejectConflict(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "UPDATE event_quarantine SET status = 'rejected', reason_code = 'event_id_hash_conflict', last_seen_at = ? WHERE tenant_id = ? AND event_id = ?", q.now, q.tenantID, q.eventID); err != nil {
		return fmt.Errorf("record quarantine hash conflict: %w", err)
	}
	return nil
}

// recordOverflow records an event gap once the record's retries are spent.
func (q quarantineRecord) recordOverflow(ctx context.Context, tx *sql.Tx) error {
	var status string
	if err := tx.QueryRowContext(ctx, "SELECT status FROM event_quarantine WHERE tenant_id = ? AND event_id = ?", q.tenantID, q.eventID).Scan(&status); err != nil {
		return fmt.Errorf("read quarantine status: %w", err)
	}
	if status != "rejected" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO event_gaps (gap_id, tenant_id, partition_id, from_position, to_position, reason_code, created_at) VALUES (?, ?, 0, 0, 0, 'quarantine_retry_exhausted', ?) ON CONFLICT(gap_id) DO NOTHING`, q.quarantineID+":gap", q.tenantID, q.now); err != nil {
		return fmt.Errorf("record quarantine overflow gap: %w", err)
	}
	return nil
}

// QuarantineEnvelope records a normalized envelope that failed validation.
func (l *EventLog) QuarantineEnvelope(ctx context.Context, tenantID string, env contractsv1.Envelope, reason, now string) error {
	encoded, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal quarantined envelope: %w", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		return fmt.Errorf("decode quarantined envelope: %w", err)
	}
	return l.Quarantine(ctx, tenantID, document, reason, now)
}

// QuarantineRaw preserves malformed JSON as data with a stable line identity.
func (l *EventLog) QuarantineRaw(ctx context.Context, tenantID, eventID string, raw []byte, reason, now string) error {
	return l.Quarantine(ctx, tenantID, map[string]any{
		"id": eventID, "type": "", "schema_version": "", "source": "",
		"data": map[string]any{"raw": string(raw)},
	}, reason, now)
}

// ReleaseQuarantine marks one record ready for an explicit re-drive.
func (l *EventLog) ReleaseQuarantine(ctx context.Context, tenantID, eventID, now string) error {
	if tenantID == "" || eventID == "" || now == "" {
		return fmt.Errorf("tenant, event, and time are required")
	}
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE event_quarantine SET status = 'released', released_at = ?, last_seen_at = ? WHERE tenant_id = ? AND event_id = ? AND status = 'quarantined'`, now, now, tenantID, eventID)
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
	}); err != nil {
		return fmt.Errorf("release quarantine transaction: %w", err)
	}
	return nil
}

// RedriveQuarantine validates and appends a released envelope atomically.
func (l *EventLog) RedriveQuarantine(ctx context.Context, tenantID, eventID, now string) (LogPosition, error) {
	if tenantID == "" || eventID == "" || now == "" {
		return -1, fmt.Errorf("tenant, event, and time are required")
	}
	position := LogPosition(-1)
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		env, err := loadReleasedEnvelope(ctx, tx, tenantID, eventID)
		if err != nil {
			return err
		}
		if err := l.admit(ctx, tx, tenantID, env); err != nil {
			return fmt.Errorf("validate released envelope: %w", err)
		}
		if position, err = l.appendOne(ctx, tx, tenantID, env); err != nil {
			return fmt.Errorf("append released event: %w", err)
		}
		return markRedriven(ctx, tx, tenantID, eventID, now)
	}); err != nil {
		return -1, fmt.Errorf("redrive quarantine transaction: %w", err)
	}
	return position, nil
}

// loadReleasedEnvelope returns a quarantined envelope that an operator
// released and that has not been redriven yet.
func loadReleasedEnvelope(ctx context.Context, tx *sql.Tx, tenantID, eventID string) (contractsv1.Envelope, error) {
	var payload []byte
	var status string
	var redrivenAt sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT payload_json, status, redriven_at FROM event_quarantine WHERE tenant_id = ? AND event_id = ?", tenantID, eventID).Scan(&payload, &status, &redrivenAt); err != nil {
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

func markRedriven(ctx context.Context, tx *sql.Tx, tenantID, eventID, now string) error {
	if _, err := tx.ExecContext(ctx, "UPDATE event_quarantine SET redriven_at = ? WHERE tenant_id = ? AND event_id = ? AND status = 'released'", now, tenantID, eventID); err != nil {
		return fmt.Errorf("mark quarantine redriven: %w", err)
	}
	return nil
}

// RecordGap records a durable discontinuity caused by bounded overflow or
// explicit operator action. It never deletes the original evidence.
func (l *EventLog) RecordGap(ctx context.Context, gapID, tenantID string, partitionID int, fromPosition, toPosition int64, reason, now string) error {
	if gapID == "" || tenantID == "" || partitionID < 0 || fromPosition < 0 || toPosition < fromPosition || reason == "" || now == "" {
		return fmt.Errorf("invalid event gap")
	}
	if _, err := l.db.ExecContext(ctx, `INSERT INTO event_gaps (gap_id, tenant_id, partition_id, from_position, to_position, reason_code, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, gapID, tenantID, partitionID, fromPosition, toPosition, reason, now); err != nil {
		return fmt.Errorf("record event gap: %w", err)
	}
	return nil
}

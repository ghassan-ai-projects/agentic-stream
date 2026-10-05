package eventlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
)

// Quarantine records an invalid event durably without placing it in the
// executable event log. Repeated delivery increments a bounded retry count.
func (l *EventLog) Quarantine(ctx context.Context, tenantID string, env map[string]any, reason, now string) error {
	if err := domain.ValidQuarantine(tenantID, reason, now); err != nil {
		return err
	}
	record, err := newQuarantineRecord(tenantID, env, reason, now)
	if err != nil {
		return err
	}
	return l.deliverQuarantine(ctx, record)
}

func newQuarantineRecord(tenantID string, env map[string]any, reason, now string) (quarantineRecord, error) {
	payload, err := domain.NewQuarantinePayload(env)
	if err != nil {
		return quarantineRecord{}, err
	}
	return quarantineRecord{payload: payload, tenantID: tenantID, reason: reason, now: now}, nil
}

func (l *EventLog) deliverQuarantine(ctx context.Context, record quarantineRecord) error {
	conflict, err := l.persistQuarantine(ctx, record)
	if err != nil {
		return err
	}
	if conflict {
		return fmt.Errorf("event id %s has conflicting quarantined payload", record.payload.EventID)
	}
	return nil
}

func (l *EventLog) persistQuarantine(ctx context.Context, record quarantineRecord) (bool, error) {
	conflict := false
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		conflict, err = record.persist(ctx, tx)
		return err
	}); err != nil {
		return false, fmt.Errorf("quarantine event transaction: %w", err)
	}
	return conflict, nil
}

// quarantineRecord is one invalid event as it is quarantined: its derived
// payload identity plus the tenant, reason and time of this delivery.
type quarantineRecord struct {
	payload  domain.QuarantinePayload
	tenantID string
	reason   string
	now      string
}

// persist inserts the record or counts a repeated delivery of the same
// payload. A different payload under the same event ID is a conflict: the
// existing record is rejected and persist reports true. A record whose retry
// count is exhausted also records an event gap.
func (q quarantineRecord) persist(ctx context.Context, tx *sql.Tx) (bool, error) {
	conflicting, err := q.conflictsWithExisting(ctx, tx)
	if err != nil {
		return false, err
	}
	if conflicting {
		return true, q.rejectConflict(ctx, tx)
	}
	updated, err := q.upsert(ctx, tx)
	if err != nil {
		return false, err
	}
	if updated == 0 {
		return true, q.rejectConflict(ctx, tx)
	}
	return false, q.recordOverflow(ctx, tx)
}

// conflictsWithExisting reports whether the event ID is already quarantined
// with a different payload.
func (q quarantineRecord) conflictsWithExisting(ctx context.Context, tx *sql.Tx) (bool, error) {
	var existingDigest []byte
	err := tx.QueryRowContext(ctx, "SELECT payload_sha256 FROM event_quarantine WHERE tenant_id = ? AND event_id = ?", q.tenantID, q.payload.EventID).Scan(&existingDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load existing quarantine: %w", err)
	}
	return q.payload.ConflictingPayload(existingDigest), nil
}

// upsert inserts the record or counts a repeated delivery of the same
// payload, returning the number of affected rows.
func (q quarantineRecord) upsert(ctx context.Context, tx *sql.Tx) (int64, error) {
	result, err := tx.ExecContext(ctx, upsertQuarantineSQL,
		q.payload.QuarantineID, q.tenantID, q.payload.EventID, q.payload.EventType, q.payload.SchemaVersion, q.payload.Source,
		q.reason, q.payload.Payload, q.payload.Digest, q.now, q.now)
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

func (q quarantineRecord) rejectConflict(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "UPDATE event_quarantine SET status = 'rejected', reason_code = 'event_id_hash_conflict', last_seen_at = ? WHERE tenant_id = ? AND event_id = ?", q.now, q.tenantID, q.payload.EventID); err != nil {
		return fmt.Errorf("record quarantine hash conflict: %w", err)
	}
	return nil
}

// recordOverflow records an event gap once the record's retries are spent.
func (q quarantineRecord) recordOverflow(ctx context.Context, tx *sql.Tx) error {
	var status string
	if err := tx.QueryRowContext(ctx, "SELECT status FROM event_quarantine WHERE tenant_id = ? AND event_id = ?", q.tenantID, q.payload.EventID).Scan(&status); err != nil {
		return fmt.Errorf("read quarantine status: %w", err)
	}
	if status != "rejected" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO event_gaps (gap_id, tenant_id, partition_id, from_position, to_position, reason_code, created_at) VALUES (?, ?, 0, 0, 0, 'quarantine_retry_exhausted', ?) ON CONFLICT(gap_id) DO NOTHING`, q.payload.OverflowGapID(), q.tenantID, q.now); err != nil {
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
	if err := domain.ValidRelease(tenantID, eventID, now); err != nil {
		return err
	}
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		return releaseQuarantined(ctx, tx, tenantID, eventID, now)
	}); err != nil {
		return fmt.Errorf("release quarantine transaction: %w", err)
	}
	return nil
}

func releaseQuarantined(ctx context.Context, tx *sql.Tx, tenantID, eventID, now string) error {
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
}

// RedriveQuarantine validates and appends a released envelope atomically.
func (l *EventLog) RedriveQuarantine(ctx context.Context, tenantID, eventID, now string) (LogPosition, error) {
	if err := domain.ValidRelease(tenantID, eventID, now); err != nil {
		return -1, err
	}
	position := LogPosition(-1)
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		position, err = l.redrive(ctx, tx, tenantID, eventID, now)
		return err
	}); err != nil {
		return -1, fmt.Errorf("redrive quarantine transaction: %w", err)
	}
	return position, nil
}

// redrive revalidates and appends a released event, then marks it redriven.
func (l *EventLog) redrive(ctx context.Context, tx *sql.Tx, tenantID, eventID, now string) (LogPosition, error) {
	env, err := loadReleasedEnvelope(ctx, tx, tenantID, eventID)
	if err != nil {
		return -1, err
	}
	if err := l.admit(ctx, tx, tenantID, env); err != nil {
		return -1, fmt.Errorf("validate released envelope: %w", err)
	}
	position, err := l.appendOne(ctx, tx, tenantID, env)
	if err != nil {
		return -1, fmt.Errorf("append released event: %w", err)
	}
	return position, markRedriven(ctx, tx, tenantID, eventID, now)
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
	if err := domain.ValidGap(gapID, tenantID, partitionID, fromPosition, toPosition, reason, now); err != nil {
		return err
	}
	if _, err := l.db.ExecContext(ctx, `INSERT INTO event_gaps (gap_id, tenant_id, partition_id, from_position, to_position, reason_code, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, gapID, tenantID, partitionID, fromPosition, toPosition, reason, now); err != nil {
		return fmt.Errorf("record event gap: %w", err)
	}
	return nil
}

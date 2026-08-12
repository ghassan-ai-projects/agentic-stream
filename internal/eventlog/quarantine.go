package eventlog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	payload, err := json.Marshal(env["data"])
	if err != nil {
		return fmt.Errorf("marshal quarantined payload: %w", err)
	}
	digest := sha256.Sum256(payload)
	idDigest := sha256.Sum256(append([]byte(eventID+"|"), payload...))
	quarantineID := "q_" + hex.EncodeToString(idDigest[:12])
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO event_quarantine (
				quarantine_id, tenant_id, event_id, event_type, schema_version, source,
				reason_code, payload_json, payload_sha256, status, first_seen_at, last_seen_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'quarantined', ?, ?)
			ON CONFLICT(tenant_id, event_id) DO UPDATE SET
				attempt_count = MIN(attempt_count + 1, 10), last_seen_at = excluded.last_seen_at,
				reason_code = excluded.reason_code, payload_json = excluded.payload_json,
				payload_sha256 = excluded.payload_sha256`,
			quarantineID, tenantID, eventID, eventType, schemaVersion, source,
			reason, payload, digest[:], now, now); err != nil {
			return fmt.Errorf("persist event quarantine: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("quarantine event transaction: %w", err)
	}
	return nil
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

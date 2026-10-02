package notify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"time"
)

// ReadPage returns up to limit events strictly after cursor. It refuses a
// resume cursor that predates retained data, and when maxLag is positive it
// disconnects a subscriber lagging further behind; both refusals are audited.
// A record that fails its digest, decoding, or validation is a poison record:
// it is skipped once its retry budget is spent and otherwise fails the page.
func ReadPage(ctx context.Context, db *storage.DB, tenantID string, cursor int64, limit int, maxLag int64, now time.Time) (Page, error) {
	if limit <= 0 || limit > 1000 {
		return Page{}, fmt.Errorf("notification limit must be between 1 and 1000")
	}
	if err := checkResumeCursor(ctx, db, tenantID, cursor, maxLag, now); err != nil {
		return Page{}, err
	}
	rawRecords, err := readRawNotifications(ctx, db, tenantID, cursor, limit)
	if err != nil {
		return Page{}, err
	}
	result := Page{Records: make([]Record, 0, limit), NextCursor: cursor}
	for _, raw := range rawRecords {
		result.NextCursor = raw.cursor
		record, valid := raw.decode(tenantID)
		if !valid {
			skip, poisonErr := recordPoisonAttempt(ctx, db, tenantID, raw.cursor, now)
			if poisonErr != nil {
				return Page{}, poisonErr
			}
			if !skip {
				return Page{}, ErrNotificationPoison
			}
			result.Skipped++
			continue
		}
		if err := clearPoisonAttempt(ctx, db, tenantID, raw.cursor); err != nil {
			return Page{}, err
		}
		result.Records = append(result.Records, record)
	}
	return result, nil
}

// checkResumeCursor refuses an expired cursor or, when maxLag is positive, a
// subscriber that lags too far behind, auditing each refusal.
func checkResumeCursor(ctx context.Context, db *storage.DB, tenantID string, cursor, maxLag int64, now time.Time) error {
	var oldest sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT MIN(cursor) FROM notifications WHERE tenant_id = ?", tenantID).Scan(&oldest); err != nil {
		return fmt.Errorf("find oldest notification: %w", err)
	}
	var nextCursor sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT next_cursor FROM notification_cursors WHERE tenant_id = ?", tenantID).Scan(&nextCursor); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read notification highwater: %w", err)
	}
	if (oldest.Valid && cursor < oldest.Int64-1) || (!oldest.Valid && nextCursor.Valid && cursor < nextCursor.Int64-1) {
		if err := audit(ctx, db, tenantID, "cursor_expired", cursor, oldest.Int64, now); err != nil {
			return fmt.Errorf("audit expired cursor: %w", err)
		}
		return ErrCursorExpired
	}
	if maxLag > 0 && nextCursor.Valid && nextCursor.Int64-1-cursor > maxLag {
		if err := audit(ctx, db, tenantID, "subscriber_too_slow", cursor, oldest.Int64, now); err != nil {
			return fmt.Errorf("audit slow subscriber: %w", err)
		}
		return ErrSubscriberTooSlow
	}
	return nil
}

type rawNotification struct {
	cursor              int64
	eventJSON, eventSHA []byte
}

// readRawNotifications reads the page rows and closes the result set before
// poison accounting writes to the database.
func readRawNotifications(ctx context.Context, db *storage.DB, tenantID string, cursor int64, limit int) ([]rawNotification, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT cursor, event_json, event_sha256 FROM notifications
		WHERE tenant_id = ? AND cursor > ? ORDER BY cursor LIMIT ?`, tenantID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("read notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()
	rawRecords := make([]rawNotification, 0, limit)
	for rows.Next() {
		var raw rawNotification
		if err := rows.Scan(&raw.cursor, &raw.eventJSON, &raw.eventSHA); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		rawRecords = append(rawRecords, raw)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close notifications: %w", err)
	}
	return rawRecords, nil
}

// decode verifies the stored digest, then decodes and validates the event.
func (raw rawNotification) decode(tenantID string) (Record, bool) {
	record := Record{TenantID: tenantID, Cursor: raw.cursor}
	computed := sha256.Sum256(raw.eventJSON)
	if !bytes.Equal(computed[:], raw.eventSHA) {
		return Record{}, false
	}
	if err := json.Unmarshal(raw.eventJSON, &record.Event); err != nil {
		return Record{}, false
	}
	if err := record.Event.Validate(); err != nil {
		return Record{}, false
	}
	return record, true
}

func recordPoisonAttempt(ctx context.Context, db *storage.DB, tenantID string, cursor int64, now time.Time) (bool, error) {
	var skip bool
	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO notification_poison_attempts (tenant_id, cursor, attempts, last_attempt_at) VALUES (?, ?, 1, ?) ON CONFLICT(tenant_id, cursor) DO UPDATE SET attempts = attempts + 1, last_attempt_at = excluded.last_attempt_at`, tenantID, cursor, now.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return fmt.Errorf("record notification poison attempt: %w", err)
		}
		var attempts int
		if err := tx.QueryRowContext(ctx, "SELECT attempts FROM notification_poison_attempts WHERE tenant_id = ? AND cursor = ?", tenantID, cursor).Scan(&attempts); err != nil {
			return fmt.Errorf("read notification poison attempts: %w", err)
		}
		if attempts < maxNotificationPoisonAttempts {
			return nil
		}
		if err := auditTx(ctx, tx, tenantID, "subscriber_skipped", cursor, cursor, now); err != nil {
			return fmt.Errorf("audit skipped poison notification: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM notification_poison_attempts WHERE tenant_id = ? AND cursor = ?", tenantID, cursor); err != nil {
			return fmt.Errorf("clear notification poison attempts: %w", err)
		}
		skip = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("persist notification poison attempt: %w", err)
	}
	return skip, nil
}

func clearPoisonAttempt(ctx context.Context, db *storage.DB, tenantID string, cursor int64) error {
	if _, err := db.ExecContext(ctx, "DELETE FROM notification_poison_attempts WHERE tenant_id = ? AND cursor = ?", tenantID, cursor); err != nil {
		return fmt.Errorf("clear notification poison attempt: %w", err)
	}
	return nil
}

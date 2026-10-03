package notify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// ReadPage returns up to limit events strictly after cursor. It refuses a
// resume cursor that predates retained data, and when maxLag is positive it
// disconnects a subscriber lagging further behind; both refusals are audited.
// A record that fails its digest, decoding, or validation is a poison record:
// it is skipped once its retry budget is spent and otherwise fails the page.
func ReadPage(ctx context.Context, db *storage.DB, tenantID string, cursor int64, limit int, maxLag int64, now time.Time) (Page, error) {
	if err := checkPageRequest(ctx, db, tenantID, cursor, limit, maxLag, now); err != nil {
		return Page{}, err
	}
	rawRecords, err := readRawNotifications(ctx, db, tenantID, cursor, limit)
	if err != nil {
		return Page{}, err
	}
	result := Page{Records: make([]Record, 0, limit), NextCursor: cursor}
	for _, raw := range rawRecords {
		if err := result.admitRecord(ctx, db, tenantID, raw, now); err != nil {
			return Page{}, err
		}
	}
	return result, nil
}

func (page *Page) admitRecord(ctx context.Context, db *storage.DB, tenantID string, raw rawNotification, now time.Time) error {
	page.NextCursor = raw.cursor
	record, valid := raw.decode(tenantID)
	if !valid {
		return page.admitPoison(ctx, db, tenantID, raw.cursor, now)
	}
	if err := clearPoisonAttempt(ctx, db, tenantID, raw.cursor); err != nil {
		return err
	}
	page.Records = append(page.Records, record)
	return nil
}

func (page *Page) admitPoison(ctx context.Context, db *storage.DB, tenantID string, cursor int64, now time.Time) error {
	skip, err := recordPoisonAttempt(ctx, db, tenantID, cursor, now)
	if err != nil {
		return err
	}
	if !skip {
		return ErrNotificationPoison
	}
	page.Skipped++
	return nil
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
	return checkSubscriberLag(ctx, db, tenantID, cursor, maxLag, oldest, nextCursor, now)
}

type rawNotification struct {
	cursor              int64
	eventJSON, eventSHA []byte
}

// readRawNotifications reads the page rows and closes the result set before
// poison accounting writes to the database.
func readRawNotifications(ctx context.Context, db *storage.DB, tenantID string, cursor int64, limit int) ([]rawNotification, error) {
	rows, err := db.QueryContext(ctx, readNotificationPageSQL, tenantID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("read notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()
	records, err := collectRawNotifications(rows, limit)
	if err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close notifications: %w", err)
	}
	return records, nil
}

const readNotificationPageSQL = `
 SELECT cursor, event_json, event_sha256 FROM notifications
 WHERE tenant_id = ? AND cursor > ? ORDER BY cursor LIMIT ?`

func collectRawNotifications(rows *sql.Rows, limit int) ([]rawNotification, error) {
	records := make([]rawNotification, 0, limit)
	for rows.Next() {
		var raw rawNotification
		if err := rows.Scan(&raw.cursor, &raw.eventJSON, &raw.eventSHA); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		records = append(records, raw)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}
	return records, nil
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

func checkSubscriberLag(ctx context.Context, db *storage.DB, tenantID string, cursor, maxLag int64, oldest, nextCursor sql.NullInt64, now time.Time) error {
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

func checkPageRequest(ctx context.Context, db *storage.DB, tenantID string, cursor int64, limit int, maxLag int64, now time.Time) error {
	if limit <= 0 || limit > 1000 {
		return fmt.Errorf("notification limit must be between 1 and 1000")
	}
	return checkResumeCursor(ctx, db, tenantID, cursor, maxLag, now)
}

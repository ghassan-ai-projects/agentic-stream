package store

import (
	"context"
	"database/sql"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type Store struct {
	DB *storage.DB
}

func New(db *storage.DB) Store { return Store{DB: db} }

type Unit struct {
	tx *sql.Tx
}

func (s Store) Unit(ctx context.Context, work func(*Unit) error) error {
	return s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return work(&Unit{tx: tx})
	})
}

func (u *Unit) LoadEventSchemaJSON(ctx context.Context, eventType, schemaVersion string) ([]byte, error) {
	var schemaJSON []byte
	if err := u.tx.QueryRowContext(ctx, "SELECT schema_json FROM event_schemas WHERE event_type = ? AND schema_version = ? AND status = 'active'", eventType, schemaVersion).Scan(&schemaJSON); err != nil {
		return nil, fmt.Errorf("event schema %s/%s is not registered: %w", eventType, schemaVersion, err)
	}
	return schemaJSON, nil
}

func (s Store) CurrentPosition(ctx context.Context, tenantID string) (domain.LogPosition, error) {
	var position int64
	if err := s.DB.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), 0) FROM event_log WHERE tenant_id = ?", tenantID).Scan(&position); err != nil {
		return 0, fmt.Errorf("read current event position for tenant %q: %w", tenantID, err)
	}
	return domain.LogPosition(position), nil
}

const quarantineListLimit = 500

func (s Store) Quarantined(ctx context.Context, tenantID string) ([]domain.QuarantineRecord, error) {
	records, err := storage.QueryAll(ctx, s.DB, "quarantine records", scanQuarantineRecord, `SELECT event_id, event_type, reason_code, status, redriven_at IS NOT NULL, attempt_count, first_seen_at, last_seen_at
		FROM event_quarantine WHERE tenant_id = ? ORDER BY last_seen_at DESC, event_id LIMIT ?`, tenantID, quarantineListLimit)
	if err != nil {
		return nil, fmt.Errorf("list quarantine: %w", err)
	}
	return records, nil
}

func scanQuarantineRecord(rows *sql.Rows) (domain.QuarantineRecord, error) {
	var record domain.QuarantineRecord
	var redriven bool
	if err := rows.Scan(&record.EventID, &record.EventType, &record.ReasonCode, &record.Status, &redriven, &record.AttemptCount, &record.FirstSeenAt, &record.LastSeenAt); err != nil {
		return domain.QuarantineRecord{}, fmt.Errorf("scan quarantine record: %w", err)
	}
	record.Status = domain.OperatorStatus(record.Status, redriven)
	return record, nil
}

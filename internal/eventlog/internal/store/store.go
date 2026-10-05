package store

import (
	"context"
	"database/sql"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Store is the only event-log layer that speaks SQL.
type Store struct {
	DB *storage.DB
}

// New wraps the evidence-log database.
func New(db *storage.DB) Store { return Store{DB: db} }

// Unit is one open transaction over the evidence log; the application layer
// sequences admission, insert and lifecycle steps inside it.
type Unit struct {
	tx *sql.Tx
}

// Unit runs work inside one transaction.
func (s Store) Unit(ctx context.Context, work func(*Unit) error) error {
	return s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return work(&Unit{tx: tx})
	})
}

// LoadEventSchemaJSON returns the active registered schema document for an
// event type and version.
func (u *Unit) LoadEventSchemaJSON(ctx context.Context, eventType, schemaVersion string) ([]byte, error) {
	var schemaJSON []byte
	if err := u.tx.QueryRowContext(ctx, "SELECT schema_json FROM event_schemas WHERE event_type = ? AND schema_version = ? AND status = 'active'", eventType, schemaVersion).Scan(&schemaJSON); err != nil {
		return nil, fmt.Errorf("event schema %s/%s is not registered: %w", eventType, schemaVersion, err)
	}
	return schemaJSON, nil
}

// CurrentPosition returns the greatest durable log position for tenantID, or
// zero when the tenant has no records.
func (s Store) CurrentPosition(ctx context.Context, tenantID string) (domain.LogPosition, error) {
	var position int64
	if err := s.DB.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), 0) FROM event_log WHERE tenant_id = ?", tenantID).Scan(&position); err != nil {
		return 0, fmt.Errorf("read current event position for tenant %q: %w", tenantID, err)
	}
	return domain.LogPosition(position), nil
}

// RecordGap inserts one durable gap record.
func (s Store) RecordGap(ctx context.Context, gap domain.Gap) error {
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO event_gaps (gap_id, tenant_id, partition_id, from_position, to_position, reason_code, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, gap.ID, gap.TenantID, gap.PartitionID, gap.From, gap.To, gap.Reason, gap.CreatedAt); err != nil {
		return fmt.Errorf("record event gap: %w", err)
	}
	return nil
}

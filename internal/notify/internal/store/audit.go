package store

import (
	"context"
	"fmt"
	"time"
)

// Audit is one notification audit row.
type Audit struct {
	ID        string
	TenantID  string
	Action    string
	Requested int64
	Oldest    int64
	Details   []byte
	At        time.Time
}

// RecordAudit appends an audit row.
func (tx *Tx) RecordAudit(ctx context.Context, audit Audit) error {
	_, err := tx.q.ExecContext(ctx, `INSERT INTO notification_audits (audit_id, tenant_id, action, requested_cursor, oldest_cursor, details_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		audit.ID, audit.TenantID, audit.Action, audit.Requested, audit.Oldest, audit.Details, formatTime(audit.At))
	if err != nil {
		return fmt.Errorf("write notification audit: %w", err)
	}
	return nil
}

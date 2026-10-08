package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// SafetyEvents reads every safety event in recording order. Each stored
// details document must still match its digest: tampered evidence fails the
// read instead of being counted.
func (t *Tx) SafetyEvents(ctx context.Context) ([]domain.SafetyEvent, error) {
	rows, err := t.tx.QueryContext(ctx, `SELECT event_id, event_type, target, COALESCE(command_id, ''), details_json, details_sha256, occurred_at
		FROM device_safety_events ORDER BY event_id`)
	if err != nil {
		return nil, fmt.Errorf("query safety events: %w", err)
	}
	events, err := storage.CollectRows(rows, "safety events", scanSafetyEvent)
	if err != nil {
		return nil, fmt.Errorf("read safety events: %w", err)
	}
	return events, nil
}

func scanSafetyEvent(rows *sql.Rows) (domain.SafetyEvent, error) {
	var row storedSafetyEvent
	if err := rows.Scan(&row.id, &row.event.Type, &row.event.Target, &row.event.CommandID, &row.details, &row.digest, &row.occurred); err != nil {
		return domain.SafetyEvent{}, fmt.Errorf("scan safety event: %w", err)
	}
	return row.decode()
}

// storedSafetyEvent is one safety-event row before its details are verified.
type storedSafetyEvent struct {
	id              int64
	event           domain.SafetyEvent
	details, digest []byte
	occurred        string
}

func (row storedSafetyEvent) decode() (domain.SafetyEvent, error) {
	if err := canonicaljson.VerifyStored(row.details, row.digest); err != nil {
		return domain.SafetyEvent{}, fmt.Errorf("verify safety event %d: %w", row.id, err)
	}
	event := row.event
	if err := json.Unmarshal(row.details, &event.Details); err != nil {
		return domain.SafetyEvent{}, fmt.Errorf("decode safety event %d: %w", row.id, err)
	}
	occurred, err := kernel.ParseTime(row.occurred)
	if err != nil {
		return domain.SafetyEvent{}, fmt.Errorf("parse safety event time: %w", err)
	}
	event.Occurred = occurred
	return event, nil
}

// CountOpenReconciliations counts devices whose reconciliation is required.
func (t *Tx) CountOpenReconciliations(ctx context.Context) (uint64, error) {
	return t.count(ctx, "open reconciliations", `SELECT COUNT(*) FROM device_reconciliation WHERE status = ?`, string(domain.ReconciliationRequired))
}

// CountAuthorityEvents counts the authority audit log.
func (t *Tx) CountAuthorityEvents(ctx context.Context) (uint64, error) {
	return t.count(ctx, "authority events", `SELECT COUNT(*) FROM device_authority_events`)
}

func (t *Tx) count(ctx context.Context, what, query string, args ...any) (uint64, error) {
	var count uint64
	if err := t.tx.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count %s: %w", what, err)
	}
	return count, nil
}

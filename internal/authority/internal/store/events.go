package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

const appendAuthorityEventSQL = `
		INSERT INTO device_authority_events
			(target, device_id, event_type, owner_epoch, owner_instance, boot_id, details_json, details_sha256, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

// AppendAuthorityEvent appends an audit record with canonical details and
// their digest.
func (t *Tx) AppendAuthorityEvent(ctx context.Context, event domain.AuthorityEvent) error {
	details, sum, err := canonicalDocument("authority event", event.Details)
	if err != nil {
		return err
	}
	subject := event.Subject
	if _, err := t.tx.ExecContext(ctx, appendAuthorityEventSQL, subject.Target, subject.Device.DeviceID,
		string(event.Type), subject.Owner.Epoch, subject.Owner.Instance, subject.Device.BootID,
		details, sum, kernel.FormatTime(event.OccurredAt)); err != nil {
		return fmt.Errorf("record authority event: %w", err)
	}
	return nil
}

// SafeStopLatched reports whether any safe-stop stage is recorded for the
// device boot.
func (s *Store) SafeStopLatched(ctx context.Context, device domain.DeviceBoot) (bool, error) {
	query, args := safeStopLatchQuery(device)
	var count int64
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return false, fmt.Errorf("read safe-stop latch: %w", err)
	}
	return count > 0, nil
}

// safeStopLatchQuery counts the boot's events of every safe-stop stage, taking
// the stage names from the domain.
func safeStopLatchQuery(device domain.DeviceBoot) (string, []any) {
	args := []any{device.DeviceID, device.BootID}
	for _, stage := range domain.SafeStopStages {
		args = append(args, string(stage))
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(domain.SafeStopStages)), ", ")
	return `SELECT COUNT(*) FROM device_authority_events
		WHERE device_id = ? AND boot_id = ? AND event_type IN (` + placeholders + `)`, args
}

const appendSafetyEventSQL = `
		INSERT INTO device_safety_events (event_type, target, command_id, details_json, details_sha256, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?)`

// AppendSafetyEvent appends one piece of safety evidence.
func (t *Tx) AppendSafetyEvent(ctx context.Context, event domain.SafetyEvent) error {
	details, sum, err := canonicalDocument("safety event", event.Details)
	if err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(ctx, appendSafetyEventSQL, string(event.Type), event.Target, nullable(event.CommandID),
		details, sum, kernel.FormatTime(event.Occurred)); err != nil {
		return fmt.Errorf("record safety event: %w", err)
	}
	return nil
}

// canonicalDocument encodes stored details as canonical JSON with their raw
// SHA-256.
func canonicalDocument(what string, document map[string]any) ([]byte, []byte, error) {
	data, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, nil, fmt.Errorf("canonicalize %s: %w", what, err)
	}
	return data, canonicaljson.Sum(data), nil
}

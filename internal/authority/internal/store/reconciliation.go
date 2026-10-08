package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// LoadReconciliation returns the recorded reconciliation of a device inside
// the unit of work, or nil when the device has never reported its state.
func (t *Tx) LoadReconciliation(ctx context.Context, deviceID string) (*domain.Reconciliation, error) {
	return loadReconciliation(ctx, t.tx, deviceID)
}

// LoadReconciliation reads the recorded reconciliation of a device outside a
// unit of work, without taking the write lock.
func (s *Store) LoadReconciliation(ctx context.Context, deviceID string) (*domain.Reconciliation, error) {
	return loadReconciliation(ctx, s.db, deviceID)
}

func loadReconciliation(ctx context.Context, r reader, deviceID string) (*domain.Reconciliation, error) {
	recorded := domain.Reconciliation{Device: domain.DeviceBoot{DeviceID: deviceID}}
	var status string
	err := r.QueryRowContext(ctx, `SELECT boot_id, status, state_sha256 FROM device_reconciliation WHERE device_id = ?`,
		deviceID).Scan(&recorded.Device.BootID, &status, &recorded.StateSHA256)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load device reconciliation: %w", err)
	}
	recorded.Status = domain.ReconciliationStatus(status)
	return &recorded, nil
}

const insertFirstStateSQL = `
		INSERT INTO device_reconciliation
			(device_id, boot_id, status, state_json, state_sha256, owner_epoch, first_seen_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

// InsertFirstState records the first state a device reports, with no open
// reconciliation.
func (t *Tx) InsertFirstState(ctx context.Context, state domain.DeviceState, owner domain.Owner, now time.Time) error {
	if _, err := t.tx.ExecContext(ctx, insertFirstStateSQL, state.Device.DeviceID, state.Device.BootID,
		string(domain.ReconciliationClear), state.JSON, state.SHA256, owner.Epoch,
		kernel.FormatTime(now), kernel.FormatTime(now)); err != nil {
		return fmt.Errorf("insert device state: %w", err)
	}
	return nil
}

// RefreshState records a newer state of the same device boot. The
// reconciliation status and any resolution are kept.
func (t *Tx) RefreshState(ctx context.Context, state domain.DeviceState, owner domain.Owner, now time.Time) error {
	if _, err := t.tx.ExecContext(ctx, `
		UPDATE device_reconciliation SET state_json = ?, state_sha256 = ?, owner_epoch = ?, updated_at = ?
		WHERE device_id = ?`,
		state.JSON, state.SHA256, owner.Epoch, kernel.FormatTime(now), state.Device.DeviceID); err != nil {
		return fmt.Errorf("refresh device state: %w", err)
	}
	return nil
}

const recordRebootSQL = `
		UPDATE device_reconciliation SET boot_id = ?, status = ?,
			state_json = ?, state_sha256 = ?, owner_epoch = ?,
			last_resolution_status = NULL, resolution_evidence_json = NULL, resolution_sha256 = NULL, resolved_at = NULL,
			updated_at = ?
		WHERE device_id = ?`

// RecordReboot records the state of a new device boot with reconciliation
// required, discarding the previous boot's resolution.
func (t *Tx) RecordReboot(ctx context.Context, state domain.DeviceState, owner domain.Owner, now time.Time) error {
	if _, err := t.tx.ExecContext(ctx, recordRebootSQL, state.Device.BootID,
		string(domain.ReconciliationRequired), state.JSON, state.SHA256, owner.Epoch, kernel.FormatTime(now),
		state.Device.DeviceID); err != nil {
		return fmt.Errorf("record device reboot: %w", err)
	}
	return nil
}

const markRequiredSQL = `
		UPDATE device_reconciliation SET status = ?,
			last_resolution_status = NULL, resolution_evidence_json = NULL, resolution_sha256 = NULL, resolved_at = NULL,
			updated_at = ?
		WHERE device_id = ? AND boot_id = ?`

// MarkRequired opens a reconciliation for the device boot, discarding any
// earlier resolution of that boot.
func (t *Tx) MarkRequired(ctx context.Context, device domain.DeviceBoot, now time.Time) error {
	if _, err := t.tx.ExecContext(ctx, markRequiredSQL, string(domain.ReconciliationRequired), kernel.FormatTime(now),
		device.DeviceID, device.BootID); err != nil {
		return fmt.Errorf("open device reconciliation: %w", err)
	}
	return nil
}

const recordResolutionSQL = `
		UPDATE device_reconciliation SET status = ?, last_resolution_status = ?,
			resolution_evidence_json = ?, resolution_sha256 = ?, resolved_at = ?, updated_at = ?
		WHERE device_id = ? AND boot_id = ?`

// RecordResolution records a resolution's outcome, evidence and resulting
// status for its device boot.
func (t *Tx) RecordResolution(ctx context.Context, resolution domain.Resolution, now time.Time) error {
	if _, err := t.tx.ExecContext(ctx, recordResolutionSQL, string(resolution.Outcome.StatusAfter()),
		string(resolution.Outcome), resolution.EvidenceJSON, resolution.EvidenceSHA256, kernel.FormatTime(now), kernel.FormatTime(now),
		resolution.Device.DeviceID, resolution.Device.BootID); err != nil {
		return fmt.Errorf("record device reconciliation resolution: %w", err)
	}
	return nil
}

// BoundCommands lists the commands bound to the device boot.
func (t *Tx) BoundCommands(ctx context.Context, device domain.DeviceBoot) ([]string, error) {
	rows, err := t.tx.QueryContext(ctx, `SELECT command_id FROM device_command_bindings WHERE device_id = ? AND boot_id = ? ORDER BY command_id`,
		device.DeviceID, device.BootID)
	if err != nil {
		return nil, fmt.Errorf("list bound commands: %w", err)
	}
	commandIDs, err := storage.CollectRows(rows, "bound commands", func(rows *sql.Rows) (string, error) {
		var commandID string
		return commandID, rows.Scan(&commandID)
	})
	if err != nil {
		return nil, fmt.Errorf("list bound commands: %w", err)
	}
	return commandIDs, nil
}

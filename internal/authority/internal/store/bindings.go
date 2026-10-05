package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

const loadBindingSQL = `
		SELECT target, device_id, boot_id, owner_epoch, owner_instance, command_sha256
		FROM device_command_bindings WHERE command_id = ?`

// LoadBinding returns the binding of a command, or nil when the command was
// never bound to a device.
func (t *Tx) LoadBinding(ctx context.Context, commandID string) (*domain.CommandBinding, error) {
	binding := domain.CommandBinding{CommandID: commandID}
	var digest []byte
	err := t.tx.QueryRowContext(ctx, loadBindingSQL, commandID).Scan(&binding.Target, &binding.Device.DeviceID,
		&binding.Device.BootID, &binding.Owner.Epoch, &binding.Owner.Instance, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load command binding: %w", err)
	}
	if len(digest) > 0 {
		binding.CommandDigest = canonicaljson.EncodeDigest(digest)
	}
	return &binding, nil
}

const insertBindingSQL = `
		INSERT INTO device_command_bindings
			(command_id, target, device_id, boot_id, owner_epoch, owner_instance, command_sha256, bound_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

// InsertBinding records a new command binding.
func (t *Tx) InsertBinding(ctx context.Context, binding domain.CommandBinding, now time.Time) error {
	digest, err := digestBytes(binding.CommandDigest)
	if err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(ctx, insertBindingSQL, binding.CommandID, binding.Target,
		binding.Device.DeviceID, binding.Device.BootID, binding.Owner.Epoch, binding.Owner.Instance,
		digest, formatTime(now)); err != nil {
		return fmt.Errorf("record command binding: %w", err)
	}
	return nil
}

// digestBytes stores an absent digest as NULL and a present one as its raw
// 32 bytes.
func digestBytes(reference string) (any, error) {
	if reference == "" {
		return nil, nil
	}
	decoded, err := canonicaljson.DecodeDigest(reference)
	if err != nil {
		return nil, fmt.Errorf("decode command binding digest: %w", err)
	}
	return decoded, nil
}

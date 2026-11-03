package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const loadBindingSQL = `
		SELECT target, COALESCE(normalized_target, ''), device_id, boot_id, owner_epoch, owner_instance, command_sha256, no_readback
		FROM device_command_bindings WHERE command_id = ?`

func (t *Tx) LoadBinding(ctx context.Context, commandID string) (*domain.CommandBinding, error) {
	binding := domain.CommandBinding{CommandID: commandID}
	var digest []byte
	err := t.tx.QueryRowContext(ctx, loadBindingSQL, commandID).Scan(&binding.Target, &binding.NormalizedTarget, &binding.Device.DeviceID,
		&binding.Device.BootID, &binding.Owner.Epoch, &binding.Owner.Instance, &digest, &binding.NoReadback)
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
			(command_id, target, normalized_target, device_id, boot_id, owner_epoch, owner_instance, command_sha256, bound_at, no_readback)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

func (t *Tx) InsertBinding(ctx context.Context, binding domain.CommandBinding, now time.Time) error {
	digest, err := digestBytes(binding.CommandDigest)
	if err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(ctx, insertBindingSQL, binding.CommandID, binding.Target, storage.NullIfEmpty(binding.NormalizedTarget),
		binding.Device.DeviceID, binding.Device.BootID, binding.Owner.Epoch, binding.Owner.Instance,
		digest, kernel.FormatTime(now), storage.BoolInt(binding.NoReadback)); err != nil {
		return fmt.Errorf("record command binding: %w", err)
	}
	return nil
}

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

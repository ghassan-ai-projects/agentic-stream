package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
)

// Append validates and appends a CloudEvent in the caller's transaction. Use
// this transaction together with the state mutation that caused the event.
func Append(ctx context.Context, tx *store.Tx, event contractsv1.CloudEvent, now time.Time) (int64, error) {
	sealed, err := domain.Seal(event, now)
	if err != nil {
		return 0, err
	}
	return appendSealed(ctx, tx, sealed, now)
}

func appendSealed(ctx context.Context, tx *store.Tx, sealed domain.Sealed, now time.Time) (int64, error) {
	if cursor, found, err := existingCursor(ctx, tx, sealed); err != nil || found {
		return cursor, err
	}
	if err := checkNotRetired(ctx, tx, sealed); err != nil {
		return 0, err
	}
	return publish(ctx, tx, sealed, now)
}

// existingCursor returns the cursor of an identical notification that was
// already appended; the same event ID with a different payload is an error.
func existingCursor(ctx context.Context, tx *store.Tx, sealed domain.Sealed) (int64, bool, error) {
	stored, found, err := tx.FindNotification(ctx, sealed.Event.TenantID, sealed.Event.ID)
	if err != nil || !found {
		return 0, false, err
	}
	if err := domain.CheckSamePayload(sealed.Event.ID, stored.SHA, sealed.SHA); err != nil {
		return 0, false, err
	}
	return stored.Cursor, true, nil
}

// checkNotRetired refuses to re-append an event that retention already
// retired.
func checkNotRetired(ctx context.Context, tx *store.Tx, sealed domain.Sealed) error {
	retired, found, err := tx.FindTombstone(ctx, sealed.Event.TenantID, sealed.Event.ID)
	if err != nil || !found {
		return err
	}
	return domain.RefuseRetired(sealed.Event.ID, retired, sealed.SHA)
}

package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
)

func publish(ctx context.Context, tx *store.Tx, sealed domain.Sealed, now time.Time) (int64, error) {
	event := sealed.Event
	cursor, err := tx.AllocateCursor(ctx, event.TenantID)
	if err != nil {
		return 0, err
	}
	inserted, err := tx.InsertNotification(ctx, notificationRow(sealed, cursor, now))
	if err != nil {
		return 0, err
	}
	if !inserted {
		if err := releaseRacedCursor(ctx, tx, sealed, cursor); err != nil {
			return 0, err
		}
	}
	return tx.NotificationCursor(ctx, event.TenantID, event.ID)
}

func notificationRow(sealed domain.Sealed, cursor int64, now time.Time) store.Notification {
	event := sealed.Event
	return store.Notification{
		TenantID: event.TenantID, EventID: event.ID, EventType: event.Type, Cursor: cursor,
		EventJSON: sealed.JSON, EventSHA: sealed.SHA, Traceparent: event.Traceparent, Tracestate: event.Tracestate,
		CreatedAt: now,
	}
}

// releaseRacedCursor gives back the cursor allocated for an insert that a
// concurrent identical append won, after checking the winner is identical.
func releaseRacedCursor(ctx context.Context, tx *store.Tx, sealed domain.Sealed, cursor int64) error {
	event := sealed.Event
	winner, err := tx.StoredSHA(ctx, event.TenantID, event.ID)
	if err != nil {
		return err
	}
	if err := domain.CheckSamePayload(event.ID, winner, sealed.SHA); err != nil {
		return err
	}
	return tx.ReleaseCursor(ctx, event.TenantID, cursor)
}

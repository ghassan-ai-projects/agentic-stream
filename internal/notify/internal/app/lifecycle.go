package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
)

// AppendLifecycleEvent builds, seals and contract-validates a lifecycle
// CloudEvent, then appends it in the caller's transaction.
func AppendLifecycleEvent(ctx context.Context, tx *store.Tx, request domain.LifecycleEvent) error {
	event, err := domain.NewLifecycleEvent(request)
	if err != nil {
		return err
	}
	if _, err := Append(ctx, tx, event, request.At.UTC()); err != nil {
		return fmt.Errorf("append lifecycle event %s: %w", event.Type, err)
	}
	return nil
}

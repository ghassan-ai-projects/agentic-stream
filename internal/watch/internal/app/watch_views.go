package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/store"
)

type WatchView = domain.WatchView

type FireView = domain.FireView

func Watch(ctx context.Context, s store.Store, tenantID, watchID string) (domain.WatchView, bool, error) {
	watch, found, err := s.Watch(ctx, tenantID, watchID)
	if err != nil {
		return domain.WatchView{}, false, fmt.Errorf("watch %s: %w", watchID, err)
	}
	return watch, found, nil
}

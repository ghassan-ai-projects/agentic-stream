package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
)

// ReadSafetyRecord reads the durable safety evidence inside the caller's unit
// of work, so it is consistent with the caller's other reads.
func ReadSafetyRecord(ctx context.Context, tx *store.Tx) (domain.SafetyRecord, error) {
	events, err := tx.SafetyEvents(ctx)
	if err != nil {
		return domain.SafetyRecord{}, err
	}
	record := domain.TallySafetyEvents(events)
	if record.OpenReconciliations, err = tx.CountOpenReconciliations(ctx); err != nil {
		return domain.SafetyRecord{}, err
	}
	if record.AuthorityEvents, err = tx.CountAuthorityEvents(ctx); err != nil {
		return domain.SafetyRecord{}, err
	}
	return record, nil
}

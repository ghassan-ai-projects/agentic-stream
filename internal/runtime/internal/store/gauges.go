package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type HealthStore struct {
	DB       *storage.DB
	TenantID string
}

func (h HealthStore) Counts(ctx context.Context) (domain.HealthCounts, error) {
	var counts domain.HealthCounts
	if err := h.DB.QueryRowContext(ctx, healthCountsSQL, h.TenantID, h.TenantID, h.TenantID).Scan(
		&counts.LogHead, &counts.AppliedPosition, &counts.PendingSchedulerItems,
		&counts.OutboxOpen, &counts.VerificationsAwaiting, &counts.VerificationsRefuted, &counts.ApplyFailures,
	); err != nil {
		return domain.HealthCounts{}, fmt.Errorf("read runtime health counts: %w", err)
	}
	if err := h.DB.QueryRowContext(ctx, "SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()").Scan(&counts.DatabaseBytes); err != nil {
		return domain.HealthCounts{}, fmt.Errorf("read database size: %w", err)
	}
	return counts, nil
}

const healthCountsSQL = `
		SELECT
			(SELECT COALESCE(MAX(position), 0) FROM event_log WHERE tenant_id = ?),
			(SELECT COALESCE(MAX(last_position), 0) FROM partition_checkpoints WHERE consumer_name = 'engine' AND tenant_id = ?),
			(SELECT COUNT(*) FROM scheduler_items WHERE tenant_id = ? AND status = 'pending'),
			(SELECT COUNT(*) FROM outbox WHERE status IN ('pending', 'leased')),
			(SELECT COUNT(*) FROM verifications WHERE status = 'awaiting'),
			(SELECT COUNT(*) FROM verifications WHERE status = 'refuted'),
			(SELECT COUNT(*) FROM apply_failures)`

package episodeledger

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

// UpsertSchedulerItem persists an admitted trigger opportunity and its
// deduplication identity.
func UpsertSchedulerItem(ctx context.Context, tx *sql.Tx, item SchedulerItem, tenantID string, dedupeKey []byte, now string) error {
	return app.UpsertSchedulerItem(ctx, store.Join(tx), item, tenantID, dedupeKey, now)
}

// MarkSchedulerItemAdmitted requires the scheduler item to still be pending.
func MarkSchedulerItemAdmitted(ctx context.Context, tx *sql.Tx, schedulerItemID string, now time.Time) error {
	return app.MarkSchedulerItemAdmitted(ctx, store.Join(tx), schedulerItemID, now)
}

// CoalesceSchedulerItems marks the trigger's open queue items replaced by newer work.
func CoalesceSchedulerItems(ctx context.Context, tx *sql.Tx, situationID, triggerName, now string) error {
	return app.CoalesceSchedulerItems(ctx, store.Join(tx), situationID, triggerName, now)
}

// CoalesceCostRejectedItem prevents a cost-rejected opportunity from blocking the queue.
func CoalesceCostRejectedItem(ctx context.Context, tx *sql.Tx, schedulerItemID string, now time.Time) error {
	return app.CoalesceCostRejectedItem(ctx, store.Join(tx), schedulerItemID, now)
}

// CoalesceSkippedItem removes an unavailable opportunity from the pending queue.
func CoalesceSkippedItem(ctx context.Context, tx *sql.Tx, schedulerItemID string, now time.Time) error {
	return app.CoalesceSkippedItem(ctx, store.Join(tx), schedulerItemID, now)
}

// NextPendingSchedulerItem returns the tenant's next pending scheduler item
// that is due at now, in queue order: not_before, then creation, then identity.
func NextPendingSchedulerItem(ctx context.Context, db *sql.DB, tenantID string, now time.Time) (string, bool, error) {
	return app.NextPendingSchedulerItem(ctx, store.Reader(db), tenantID, now)
}

// Scheduling reads what became of one trigger evaluation: its scheduler
// item, the episode it admitted and the results refused for that episode.
// found is false when the evaluation never created a scheduler item.
func Scheduling(ctx context.Context, db *sql.DB, tenantID, triggerID string) (SchedulingRecord, bool, error) {
	return app.Scheduling(ctx, store.Reader(db), tenantID, triggerID)
}

// Episode reads one of the tenant's episodes with its attempts and the
// results the ledger refused for it.
func Episode(ctx context.Context, db *sql.DB, tenantID, episodeID string) (EpisodeRecord, error) {
	return app.Episode(ctx, store.Reader(db), tenantID, episodeID)
}

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
func UpsertSchedulerItem(ctx context.Context, tx *sql.Tx, item SchedulerItem, tenantID string, dedupeKey []byte, now time.Time) error {
	return app.UpsertSchedulerItem(ctx, store.Join(tx), item, tenantID, dedupeKey, now)
}

// MarkSchedulerItemAdmitted requires the scheduler item to still be pending.
func MarkSchedulerItemAdmitted(ctx context.Context, tx *sql.Tx, schedulerItemID string, now time.Time) error {
	return app.MarkSchedulerItemAdmitted(ctx, store.Join(tx), schedulerItemID, now)
}

// CoalesceSchedulerItems marks the trigger's open queue items replaced by newer
// work and returns the items it changed in identity order.
func CoalesceSchedulerItems(ctx context.Context, tx *sql.Tx, situationID, triggerName string, now time.Time) ([]CoalescedItem, error) {
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

// DueSchedulerItems lists the tenant's pending scheduler items whose admission
// window is open at now, in queue order: not_before, then creation, then
// identity. An item is due when the later of its creation and not_before has
// arrived and still precedes its expiry. An item whose times are unreadable is
// left out.
func DueSchedulerItems(ctx context.Context, tx *sql.Tx, tenantID string, now time.Time) ([]DueItem, error) {
	return app.DueSchedulerItems(ctx, store.Join(tx), tenantID, now)
}

// ExpireSchedulerItem removes a pending item that can no longer be admitted,
// because it passed its expiry or its times are unreadable.
func ExpireSchedulerItem(ctx context.Context, tx *sql.Tx, schedulerItemID string, now time.Time) error {
	return app.ExpireSchedulerItem(ctx, store.Join(tx), schedulerItemID, now)
}

// PollSchedulerQueue reads the tenant's pending queue at now: the next item
// that may be admitted and the items that can never be admitted, each with
// the reason. An item whose times are unreadable is reported as expired and
// never blocks the others.
func PollSchedulerQueue(ctx context.Context, db *sql.DB, tenantID string, now time.Time) (QueuePoll, error) {
	return app.PollSchedulerQueue(ctx, store.Reader(db), tenantID, now)
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

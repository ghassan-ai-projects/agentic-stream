package store_test

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func TestSchedulingReadsTheItemOrReportsNone(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		if _, found, err := tx.Scheduling(ctx, "default", "trg_missing"); err != nil || found {
			t.Fatalf("an unscheduled trigger was found: found=%t err=%v", found, err)
		}
		if err := tx.UpsertSchedulerItem(ctx, item("sch_1", "trg_1"), "default", make32(1), at); err != nil {
			t.Fatal(err)
		}
		record, found, err := tx.Scheduling(ctx, "default", "trg_1")
		if err != nil || !found || record.SchedulerItemID != "sch_1" || record.Status != "pending" || record.EpisodeID != "" || record.Rejections == nil {
			t.Fatalf("scheduling = %+v found=%t err=%v", record, found, err)
		}
	})
}

func make32(seed byte) []byte {
	key := make([]byte, 32)
	key[0] = seed
	return key
}

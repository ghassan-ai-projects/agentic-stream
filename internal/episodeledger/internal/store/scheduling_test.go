package store_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func TestSchedulingReportsNoRecordForATriggerThatNeverQueuedAnItem(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		if _, found, err := tx.Scheduling(ctx, "default", "trg_missing"); err != nil || found {
			t.Fatalf("an unscheduled trigger was found: found=%t err=%v", found, err)
		}
	})
}

func TestSchedulingOfAnItemWithoutAnEpisodeHasNoEpisodeAndNoRejections(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		must(t, tx.UpsertSchedulerItem(ctx, item("sch_1", "trg_1"), "default", make32(1), at))
		record, found, err := tx.Scheduling(ctx, "default", "trg_1")
		want := domain.SchedulingRecord{SchedulerItemID: "sch_1", Status: "pending", Lane: "fast", ExpiresAt: ts(time.Hour), UpdatedAt: ts(0)}
		if err != nil || !found || record.Rejections == nil || len(record.Rejections) != 0 {
			t.Fatalf("scheduling = %+v found=%t err=%v, want a record with an empty rejection list", record, found, err)
		}
		record.Rejections = nil
		if record.SchedulerItemID != want.SchedulerItemID || record.Status != want.Status || record.Lane != want.Lane || record.ExpiresAt != want.ExpiresAt || record.UpdatedAt != want.UpdatedAt || record.EpisodeID != "" {
			t.Fatalf("scheduling = %+v, want %+v", record, want)
		}
		if _, found, _ := tx.Scheduling(ctx, "other-tenant", "trg_1"); found {
			t.Fatal("another tenant read the scheduling record")
		}
	})
}

func TestSchedulingExplainsWhatBecameOfAnItemItsEpisodeAndTheRefusedResults(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		must(t, tx.UpsertSchedulerItem(ctx, item("item-e1", "trg_1"), "t", make32(1), at))
		_, err := tx.AdmitPendingSchedulerItem(ctx, "item-e1", at.Add(time.Minute))
		must(t, err)
		admit(t, ctx, tx, "e1")
		for _, r := range []domain.Rejection{
			{ID: "rej_late", EpisodeID: "e1", Fence: 1, Reason: domain.RejectStaleAttempt, Details: []byte("{}"), At: at.Add(3 * time.Minute)},
			{ID: "rej_early", EpisodeID: "e1", Fence: 1, Reason: domain.RejectSchemaInvalid, Details: []byte("{}"), At: at.Add(2 * time.Minute)},
		} {
			must(t, tx.InsertRejection(ctx, r, true))
		}
		record, found, err := tx.Scheduling(ctx, "t", "trg_1")
		if err != nil || !found {
			t.Fatalf("found=%v err=%v", found, err)
		}
		if record.Status != "admitted" || record.UpdatedAt != ts(time.Minute) || record.EpisodeID != "e1" || record.EpisodeStatus != "admitted" || record.EpisodeSituationVersion != 1 {
			t.Fatalf("scheduling = %+v, want the admitted item with its admitted episode", record)
		}
		if len(record.Rejections) != 2 || record.Rejections[0].Reason != "schema_invalid" || record.Rejections[1].Reason != "stale_attempt" {
			t.Fatalf("rejections = %+v, want them oldest first", record.Rejections)
		}
	})
}

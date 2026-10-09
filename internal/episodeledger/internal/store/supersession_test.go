package store_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

const lifecycleSQL = "SELECT lifecycle_status || '|' || COALESCE(ended_at, '-') FROM episodes WHERE episode_id = ?"

func TestAKilledEpochSupersedesOnlyItsLiveEpisodes(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		for _, id := range []string{"e-admitted", "e-running", "e-concluded", "e-other-epoch"} {
			admit(t, ctx, tx, id)
		}
		execSQL(t, ctx, raw, "UPDATE episodes SET policy_epoch = 'dead' WHERE episode_id != 'e-other-epoch'")
		execSQL(t, ctx, raw, "UPDATE episodes SET lifecycle_status = 'running' WHERE episode_id = 'e-running'")
		execSQL(t, ctx, raw, "UPDATE episodes SET lifecycle_status = 'concluded', ended_at = '2026-08-01T00:00:00Z' WHERE episode_id = 'e-concluded'")
		must(t, tx.SupersedeEpochEpisodes(ctx, "dead", at))
		for id, want := range map[string]string{
			"e-admitted":    "superseded|" + ts(0),
			"e-running":     "superseded|" + ts(0),
			"e-concluded":   "concluded|2026-08-01T00:00:00Z",
			"e-other-epoch": "admitted|-",
		} {
			if got := queryText(t, ctx, raw, lifecycleSQL, id); got != want {
				t.Errorf("%s = %s, want %s", id, got, want)
			}
		}
	})
}

func TestCoalescedItemsSupersedeTheirLiveEpisodesAndCancelTheirInFlightAttempts(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		seedCoalescedSituation(t, ctx, tx, raw)
		must(t, tx.SupersedeCoalescedEpisodes(ctx, "s-shared", at))
		for id, want := range map[string]string{
			"e-coalesced-live":   "superseded|" + ts(0),
			"e-coalesced-closed": "concluded|-",
			"e-pending-item":     "running|-",
			"e-elsewhere":        "running|-",
		} {
			if got := queryText(t, ctx, raw, lifecycleSQL, id); got != want {
				t.Errorf("%s = %s, want %s", id, got, want)
			}
		}
		must(t, tx.CancelCoalescedAttempts(ctx, "s-shared"))
		for attempt, want := range map[string]string{
			"a-dispatched": string(domain.AttemptCancelling),
			"a-running":    string(domain.AttemptCancelling),
			"a-produced":   "produced",
			"a-pending":    "running",
			"a-elsewhere":  "running",
		} {
			if got := queryText(t, ctx, raw, "SELECT status FROM episode_attempts WHERE attempt_id = ?", attempt); got != want {
				t.Errorf("attempt %s = %s, want %s", attempt, got, want)
			}
		}
	})
}

func seedCoalescedSituation(t *testing.T, ctx context.Context, tx *store.Tx, raw *sql.Tx) {
	t.Helper()
	for i, s := range []struct{ episode, situation, item, itemStatus, lifecycle string }{
		{"e-coalesced-live", "s-shared", "item-1", "coalesced", "running"},
		{"e-coalesced-closed", "s-shared", "item-2", "coalesced", "concluded"},
		{"e-pending-item", "s-shared", "item-3", "pending", "running"},
		{"e-elsewhere", "s-other", "item-4", "coalesced", "running"},
	} {
		episode := admitted(s.episode)
		episode.SchedulerItemID = s.item
		must(t, tx.InsertEpisode(ctx, episode, at))
		execSQL(t, ctx, raw, "UPDATE episodes SET lifecycle_status = ? WHERE episode_id = ?", s.lifecycle, s.episode)
		execSQL(t, ctx, raw, `INSERT INTO scheduler_items (scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version, kind, lane, priority, status, dedupe_key, expires_at, created_at, updated_at)
			VALUES (?, ?, 't', ?, 1, 'standard', 'fast', 1, ?, ?, 'later', 'now', 'now')`, s.item, "trg-"+s.item, s.situation, s.itemStatus, make32(byte(i+1)))
	}
	for i, a := range []struct{ attempt, episode, status string }{
		{"a-dispatched", "e-coalesced-live", "dispatched"},
		{"a-running", "e-coalesced-live", "running"},
		{"a-produced", "e-coalesced-live", "produced"},
		{"a-pending", "e-pending-item", "running"},
		{"a-elsewhere", "e-elsewhere", "running"},
	} {
		execSQL(t, ctx, raw, "INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, started_at) VALUES (?, ?, ?, ?, 'now')", a.attempt, a.episode, i+1, a.status)
	}
}

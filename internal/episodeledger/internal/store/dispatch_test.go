package store_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func TestTheNextDispatchableEpisodeIsTheOldestLiveOneOfTheTenant(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		for id, accepted := range map[string]time.Time{
			"e-closed": at.Add(-3 * time.Hour), "e-b-tie": at.Add(-time.Hour), "e-a-tie": at.Add(-time.Hour), "e-late": at,
		} {
			must(t, tx.InsertEpisode(ctx, admitted(id), accepted))
		}
		execSQL(t, ctx, raw, "UPDATE episodes SET lifecycle_status = 'concluded' WHERE episode_id = 'e-closed'")
		execSQL(t, ctx, raw, "UPDATE episodes SET stale_rebind_count = 2, lifecycle_status = 'running' WHERE episode_id = 'e-a-tie'")
		for _, tc := range []struct {
			name, tenant, want string
			rebinds            int
		}{
			{"oldest live wins, ties by id", "t", "e-a-tie", 2},
			{"another tenant has none", "other", "", 0},
		} {
			got, found, err := tx.NextDispatchableEpisode(ctx, tc.tenant, false)
			if err != nil || found != (tc.want != "") || got.EpisodeID != tc.want || got.StaleRebindCount != tc.rebinds {
				t.Errorf("%s: %+v found=%v err=%v", tc.name, got, found, err)
			}
		}
	})
}

func TestSupersededEpisodesWithoutAnAttemptUnderAKilledEpochAreDispatchableOnlyOnRequest(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		for id, accepted := range map[string]time.Time{"e-live": at, "e-killed": at.Add(-2 * time.Hour), "e-killed-started": at.Add(-3 * time.Hour), "e-alive-epoch": at.Add(-4 * time.Hour)} {
			must(t, tx.InsertEpisode(ctx, admitted(id), accepted))
		}
		execSQL(t, ctx, raw, "UPDATE episodes SET lifecycle_status = 'superseded', policy_epoch = 'dead' WHERE episode_id IN ('e-killed', 'e-killed-started')")
		execSQL(t, ctx, raw, "UPDATE episodes SET current_attempt_id = 'a-1', current_fence = 1 WHERE episode_id = 'e-killed-started'")
		execSQL(t, ctx, raw, "UPDATE episodes SET lifecycle_status = 'superseded', policy_epoch = 'alive' WHERE episode_id = 'e-alive-epoch'")
		execSQL(t, ctx, raw, "INSERT INTO epoch_control (epoch, state, updated_at) VALUES ('dead', 'killed', '2026-01-01T00:00:00Z'), ('alive', 'draining', '2026-01-01T00:00:00Z')")
		for _, tc := range []struct {
			killed bool
			want   string
		}{{false, "e-live"}, {true, "e-killed"}} {
			got, found, err := tx.NextDispatchableEpisode(ctx, "t", tc.killed)
			if err != nil || !found || got.EpisodeID != tc.want {
				t.Errorf("killedSuperseded=%v: got %q found=%v err=%v, want %q", tc.killed, got.EpisodeID, found, err, tc.want)
			}
		}
	})
}

func TestReasoningIsInFlightWhileAnEpisodeIsLiveOrAnItemMayStartAtOnce(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		inFlight := func() bool {
			found, err := tx.ReasoningInFlight(ctx)
			must(t, err)
			return found
		}
		if inFlight() {
			t.Fatal("an empty ledger reported reasoning in flight")
		}
		admit(t, ctx, tx, "e1")
		if !inFlight() {
			t.Fatal("an admitted episode was not reasoning in flight")
		}
		execSQL(t, ctx, raw, "UPDATE episodes SET lifecycle_status = 'running' WHERE episode_id = 'e1'")
		if !inFlight() {
			t.Fatal("a running episode was not reasoning in flight")
		}
		for _, status := range []string{"concluded", "superseded", "expired", "abandoned"} {
			execSQL(t, ctx, raw, "UPDATE episodes SET lifecycle_status = ? WHERE episode_id = 'e1'", status)
			if inFlight() {
				t.Fatalf("a %s episode was reasoning in flight", status)
			}
		}
	})
}

func TestAPendingItemIsReasoningInFlightUnlessAWindowHoldsItBack(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		execSQL(t, ctx, raw, `INSERT INTO trigger_evaluations (
			trigger_id,tenant_id,deployment_id,trigger_name,situation_id,situation_version,score,threshold,lane,outcome,reasons_json,policy_sha256,evaluated_at
			) VALUES ('trg','t','d','alarm','s',1,1,0,'fast','admitted',X'5B5D',?,'now')`, make32(1))
		must(t, tx.UpsertSchedulerItem(ctx, item("i1", "trg"), "t", make32(2), at))
		for _, tc := range []struct {
			name, notBefore string
			want            bool
		}{{"due at once", "NULL", true}, {"held by a window", "'" + ts(time.Hour) + "'", false}} {
			execSQL(t, ctx, raw, "UPDATE scheduler_items SET not_before = "+tc.notBefore)
			if got, err := tx.ReasoningInFlight(ctx); err != nil || got != tc.want {
				t.Errorf("%s: in flight = %v err=%v, want %v", tc.name, got, err, tc.want)
			}
		}
		execSQL(t, ctx, raw, "UPDATE scheduler_items SET not_before = NULL, status = 'admitted'")
		if got, err := tx.ReasoningInFlight(ctx); err != nil || got {
			t.Errorf("an admitted item alone: in flight = %v err=%v, want false", got, err)
		}
	})
}

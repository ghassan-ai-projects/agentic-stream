package store_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

const attemptRowSQL = "SELECT status || '|' || COALESCE(owner_epoch, '-') || '|' || COALESCE(started_at, '-') || '|' || COALESCE(ended_at, '-') || '|' || COALESCE(CAST(terminal_json AS TEXT), '-') FROM episode_attempts WHERE attempt_id = ?"

func TestAStartedAttemptIsDispatchedAndMarksTheEpisodeRunningUnderItsFence(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		must(t, tx.InsertAttempt(ctx, domain.Identity{EpisodeID: "e1", AttemptID: "a-unowned", Fence: 1}, at))
		must(t, tx.InsertAttempt(ctx, domain.Identity{EpisodeID: "e1", AttemptID: "a-owned", Fence: 2, OwnerEpoch: "epoch"}, at))
		for id, want := range map[string]string{
			"a-unowned": "dispatched|-|" + ts(0) + "|-|-",
			"a-owned":   "dispatched|epoch|" + ts(0) + "|-|-",
		} {
			if got := queryText(t, ctx, raw, attemptRowSQL, id); got != want {
				t.Errorf("attempt %s row = %s, want %s", id, got, want)
			}
		}
		must(t, tx.RecordEpisodeAttempt(ctx, domain.Identity{EpisodeID: "e1", AttemptID: "a-owned", Fence: 2}, at.Add(time.Hour)))
		episode := queryText(t, ctx, raw, "SELECT lifecycle_status || '|' || current_attempt_id || '|' || current_fence || '|' || started_at FROM episodes WHERE episode_id = 'e1'")
		if want := "running|a-owned|2|" + ts(time.Hour); episode != want {
			t.Fatalf("episode row = %s, want %s", episode, want)
		}
		must(t, tx.RecordEpisodeAttempt(ctx, domain.Identity{EpisodeID: "e1", AttemptID: "a-later", Fence: 3}, at.Add(2*time.Hour)))
		if started := queryText(t, ctx, raw, "SELECT started_at FROM episodes WHERE episode_id = 'e1'"); started != ts(time.Hour) {
			t.Fatalf("a later attempt moved the episode's first start to %s", started)
		}
	})
}

func TestTwoAttemptsCannotShareAnIdOrAFenceOfOneEpisode(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		admit(t, ctx, tx, "e1")
		startAttempt(t, ctx, tx, "e1", "a1", 1)
		if err := tx.InsertAttempt(ctx, domain.Identity{EpisodeID: "e1", AttemptID: "a1", Fence: 2}, at); err == nil {
			t.Error("a second attempt reused an attempt id")
		}
		if err := tx.InsertAttempt(ctx, domain.Identity{EpisodeID: "e1", AttemptID: "a2", Fence: 1}, at); err == nil {
			t.Error("a second attempt reused a fence of the same episode")
		}
		if err := tx.InsertAttempt(ctx, domain.Identity{EpisodeID: "e1", AttemptID: "a0", Fence: 0}, at); err == nil {
			t.Error("fence 0 was accepted for an attempt")
		}
	})
}

func TestAnAttemptMovesToRunningKeepingItsFirstStartTime(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		identity := startAttempt(t, ctx, tx, "e1", "a1", 1)
		rows, err := tx.StartRunningAttempt(ctx, identity, at.Add(time.Hour))
		if err != nil || rows != 1 {
			t.Fatalf("StartRunningAttempt rows=%d err=%v", rows, err)
		}
		if got, want := queryText(t, ctx, raw, attemptRowSQL, "a1"), "running|-|"+ts(0)+"|-|-"; got != want {
			t.Fatalf("running attempt = %s, want %s", got, want)
		}
	})
}

func TestAnAttemptFinishesWithItsTerminalDocumentAndEndTime(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		identity := startAttempt(t, ctx, tx, "e1", "a1", 1)
		rows, err := tx.FinishAttempt(ctx, identity, domain.AttemptTimedOut, at.Add(time.Minute), []byte(`{"why":"grace"}`))
		if err != nil || rows != 1 {
			t.Fatalf("FinishAttempt rows=%d err=%v", rows, err)
		}
		if got, want := queryText(t, ctx, raw, attemptRowSQL, "a1"), "timed_out|-|"+ts(0)+"|"+ts(time.Minute)+`|{"why":"grace"}`; got != want {
			t.Fatalf("finished attempt = %s, want %s", got, want)
		}
	})
}

func TestAnAttemptStatusChangesWithoutTouchingItsTimes(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		identity := startAttempt(t, ctx, tx, "e1", "a1", 1)
		rows, err := tx.SetAttemptStatus(ctx, identity, domain.AttemptCancelling)
		if err != nil || rows != 1 {
			t.Fatalf("SetAttemptStatus rows=%d err=%v", rows, err)
		}
		if got, want := queryText(t, ctx, raw, attemptRowSQL, "a1"), string(domain.AttemptCancelling)+"|-|"+ts(0)+"|-|-"; got != want {
			t.Fatalf("canceling attempt = %s, want %s", got, want)
		}
	})
}

func TestEveryAttemptWriteIsFencedByTheAttemptsEpisodeAndFence(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		startAttempt(t, ctx, tx, "e1", "a1", 1)
		for name, wrong := range map[string]domain.Identity{
			"stale fence":   {EpisodeID: "e1", AttemptID: "a1", Fence: 0},
			"newer fence":   {EpisodeID: "e1", AttemptID: "a1", Fence: 2},
			"other episode": {EpisodeID: "e2", AttemptID: "a1", Fence: 1},
			"other attempt": {EpisodeID: "e1", AttemptID: "a2", Fence: 1},
		} {
			writes := map[string]func() (int64, error){
				"start running": func() (int64, error) { return tx.StartRunningAttempt(ctx, wrong, at) },
				"set status":    func() (int64, error) { return tx.SetAttemptStatus(ctx, wrong, domain.AttemptCancelling) },
				"finish":        func() (int64, error) { return tx.FinishAttempt(ctx, wrong, domain.AttemptFailed, at, nil) },
			}
			for write, run := range writes {
				if rows, err := run(); err != nil || rows != 0 {
					t.Errorf("%s with %s: rows=%d err=%v, want no row changed", write, name, rows, err)
				}
			}
		}
		if got, want := queryText(t, ctx, raw, attemptRowSQL, "a1"), "dispatched|-|"+ts(0)+"|-|-"; got != want {
			t.Fatalf("a fenced-out write changed the attempt: %s, want %s", got, want)
		}
	})
}

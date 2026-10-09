package store_test

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func seedAttempt(t *testing.T, ctx context.Context, raw *sql.Tx, id, episode string, fence int, status string, owner any) {
	t.Helper()
	execSQL(t, ctx, raw, "INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, owner_epoch, started_at) VALUES (?, ?, ?, ?, ?, 'now')", id, episode, fence, status, owner)
}

func TestUnfinishedAttemptsAreTheActiveOnesOfAnyOtherEpoch(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		seedAttempt(t, ctx, raw, "a-old-dispatched", "e1", 1, "dispatched", "old")
		seedAttempt(t, ctx, raw, "a-old-canceling", "e1", 2, string(domain.AttemptCancelling), "old")
		seedAttempt(t, ctx, raw, "a-unowned-running", "e1", 3, "running", nil)
		seedAttempt(t, ctx, raw, "a-current-running", "e1", 4, "running", "current")
		seedAttempt(t, ctx, raw, "a-old-produced", "e1", 5, "produced", "old")
		seedAttempt(t, ctx, raw, "a-old-abandoned", "e1", 6, "abandoned", "old")
		attempts, err := tx.UnfinishedAttempts(ctx, "current")
		if err != nil {
			t.Fatal(err)
		}
		slices.SortFunc(attempts, func(a, b domain.UnfinishedAttempt) int { return cmp.Compare(a.AttemptID, b.AttemptID) })
		want := []domain.UnfinishedAttempt{
			{AttemptID: "a-old-canceling", EpisodeID: "e1", Status: domain.AttemptCancelling, OwnerEpoch: "old"},
			{AttemptID: "a-old-dispatched", EpisodeID: "e1", Status: domain.AttemptDispatched, OwnerEpoch: "old"},
			{AttemptID: "a-unowned-running", EpisodeID: "e1", Status: domain.AttemptRunning},
		}
		if !slices.Equal(attempts, want) {
			t.Fatalf("unfinished attempts = %+v, want %+v", attempts, want)
		}
	})
}

func TestAbandoningAnUnfinishedAttemptRecordsTheTerminalOnceAndOnlyWhileUnfinished(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		seedAttempt(t, ctx, raw, "a-running", "e1", 1, "running", "old")
		seedAttempt(t, ctx, raw, "a-produced", "e1", 2, "produced", "old")
		rows, err := tx.AbandonUnfinishedAttempt(ctx, "a-running", "e1", at, []byte(`{"reason":"runtime_restart"}`))
		if err != nil || rows != 1 {
			t.Fatalf("first abandon rows=%d err=%v", rows, err)
		}
		if got, want := queryText(t, ctx, raw, attemptRowSQL, "a-running"), "abandoned|old|now|"+ts(0)+`|{"reason":"runtime_restart"}`; got != want {
			t.Fatalf("abandoned attempt = %s, want %s", got, want)
		}
		for _, tc := range []struct{ name, attempt, episode string }{
			{"again", "a-running", "e1"}, {"terminal attempt", "a-produced", "e1"}, {"other episode", "a-running", "e2"},
		} {
			if rows, err := tx.AbandonUnfinishedAttempt(ctx, tc.attempt, tc.episode, at, []byte("{}")); err != nil || rows != 0 {
				t.Errorf("%s: rows=%d err=%v, want no row changed", tc.name, rows, err)
			}
		}
		if status := queryText(t, ctx, raw, "SELECT status FROM episode_attempts WHERE attempt_id = 'a-produced'"); status != "produced" {
			t.Fatalf("a terminal attempt was rewritten to %s", status)
		}
	})
}

func TestAbandoningAnOpenEpisodeLeavesAClosedEpisodeAlone(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e-live")
		admit(t, ctx, tx, "e-closed")
		must(t, tx.ConcludeEpisode(ctx, "e-closed", at, []byte(`{"status":"produced"}`)))
		if rows, err := tx.AbandonOpenEpisode(ctx, "e-live", at, []byte(`{"reason":"runtime_restart"}`)); err != nil || rows != 1 {
			t.Fatalf("open episode rows=%d err=%v", rows, err)
		}
		if rows, err := tx.AbandonOpenEpisode(ctx, "e-closed", at, []byte("{}")); err != nil || rows != 0 {
			t.Fatalf("closed episode rows=%d err=%v", rows, err)
		}
		for id, want := range map[string]string{"e-live": "abandoned", "e-closed": "concluded"} {
			if got := queryText(t, ctx, raw, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", id); got != want {
				t.Errorf("%s = %s, want %s", id, got, want)
			}
		}
		if terminal := queryText(t, ctx, raw, "SELECT CAST(terminal_json AS TEXT) FROM episodes WHERE episode_id = 'e-closed'"); terminal != `{"status":"produced"}` {
			t.Fatalf("closed episode terminal rewritten: %s", terminal)
		}
	})
}

type recordingSettler struct {
	episodes []string
	actual   []uint64
	now      []string
	onTx     *sql.Tx
	fail     error
}

func (s *recordingSettler) Settle(_ context.Context, tx *sql.Tx, episodeID string, actual uint64, now string) error {
	s.episodes, s.actual, s.now, s.onTx = append(s.episodes, episodeID), append(s.actual, actual), append(s.now, now), tx
	return s.fail
}

func TestReleasingAnEpisodesCostSettlesZeroOnTheCallersTransaction(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		settler := &recordingSettler{}
		must(t, tx.SettleEpisodeCost(ctx, settler, "e1", at))
		if !slices.Equal(settler.episodes, []string{"e1"}) || settler.actual[0] != 0 || settler.now[0] != ts(0) || settler.onTx != raw {
			t.Fatalf("settler saw episodes=%v actual=%v now=%v onCallersTx=%v", settler.episodes, settler.actual, settler.now, settler.onTx == raw)
		}
	})
}

func TestACostSettlementFailureNamesTheEpisodeAndKeepsItsCause(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		cause := errors.New("ledger closed")
		err := tx.SettleEpisodeCost(ctx, &recordingSettler{fail: cause}, "e1", at)
		if !errors.Is(err, cause) || err.Error() != "settle cost of episode e1: ledger closed" {
			t.Fatalf("SettleEpisodeCost = %v, want the cause wrapped with the episode", err)
		}
	})
}

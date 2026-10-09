package store_test

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func rejection(id, episode, attempt string, fence int64, reason domain.RejectionReason, when time.Time) domain.Rejection {
	return domain.Rejection{ID: id, EpisodeID: episode, AttemptID: attempt, Fence: fence, Reason: reason, Details: []byte(`{"id":"` + id + `"}`), At: when}
}

func TestEpisodeExistsOnlyForTheEpisodesTheLedgerHolds(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		if known, err := tx.EpisodeExists(ctx, "e1"); err != nil || known {
			t.Fatalf("before admission: known=%v err=%v", known, err)
		}
		admit(t, ctx, tx, "e1")
		if known, err := tx.EpisodeExists(ctx, "e1"); err != nil || !known {
			t.Fatalf("after admission: known=%v err=%v", known, err)
		}
	})
}

func TestARejectionIsRecordedOnceAndKeepsItsReference(t *testing.T) {
	t.Parallel()
	const rowSQL = "SELECT COALESCE(episode_id, '-') || '|' || COALESCE(attempt_id, '-') || '|' || fence || '|' || reason || '|' || CAST(details_json AS TEXT) || '|' || created_at FROM episode_rejections WHERE rejection_id = ?"
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		startAttempt(t, ctx, tx, "e1", "a1", 1)
		known := rejection("rej_known", "e1", "a1", 1, domain.RejectStaleAttempt, at)
		unknown := rejection("rej_unknown", "forged", "", 9, domain.RejectUnknownEpisode, at)
		for range 2 {
			must(t, tx.InsertRejection(ctx, known, true))
			must(t, tx.InsertRejection(ctx, unknown, false))
		}
		for id, want := range map[string]string{
			"rej_known":   `e1|a1|1|stale_attempt|{"id":"rej_known"}|` + ts(0),
			"rej_unknown": `-|-|9|unknown_episode|{"id":"rej_unknown"}|` + ts(0),
		} {
			if got := queryText(t, ctx, raw, rowSQL, id); got != want {
				t.Errorf("rejection %s = %s, want %s", id, got, want)
			}
		}
		if count := queryText(t, ctx, raw, "SELECT COUNT(*) FROM episode_rejections"); count != "2" {
			t.Fatalf("rejection rows = %s, want 2 after repeating each insert", count)
		}
	})
}

func TestAnEpisodesRejectionsReadOldestFirstThenById(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		admit(t, ctx, tx, "e1")
		admit(t, ctx, tx, "e2")
		for _, r := range []domain.Rejection{
			rejection("rej_c", "e1", "", 1, domain.RejectSchemaInvalid, at.Add(time.Minute)),
			rejection("rej_b", "e1", "", 1, domain.RejectOversized, at),
			rejection("rej_a", "e1", "", 1, domain.RejectExpired, at),
			rejection("rej_other", "e2", "", 1, domain.RejectExpired, at),
		} {
			must(t, tx.InsertRejection(ctx, r, true))
		}
		got, err := tx.Rejections(ctx, "e1")
		if err != nil {
			t.Fatal(err)
		}
		var reasons []string
		for _, r := range got {
			reasons = append(reasons, r.Reason)
		}
		if want := []string{"expired", "oversized", "schema_invalid"}; !slices.Equal(reasons, want) {
			t.Fatalf("rejection order = %v, want %v", reasons, want)
		}
		if first := got[0]; first.AttemptID != "" || first.Fence != 1 || string(first.Details) != `{"id":"rej_a"}` || first.CreatedAt != ts(0) {
			t.Fatalf("first rejection = %+v", first)
		}
		if none, err := tx.Rejections(ctx, "unknown"); err != nil || none == nil || len(none) != 0 {
			t.Fatalf("rejections of an unknown episode = %#v err=%v, want an empty list that encodes as []", none, err)
		}
	})
}

package store_test

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

const episodeStateSQL = `SELECT lifecycle_status || '|' || situation_version || '|' || stale_rebind_count || '|' || COALESCE(ended_at, '-') || '|' ||
	COALESCE(CAST(terminal_json AS TEXT), '-') || '|' || CAST(request_json AS TEXT) || '|' || hex(snapshot_sha256) FROM episodes WHERE episode_id = 'e1'`

func episodeState(t *testing.T, ctx context.Context, raw *sql.Tx) string {
	t.Helper()
	return queryText(t, ctx, raw, episodeStateSQL)
}

func digestHex(seed byte) string { return strings.ToUpper(hex.EncodeToString(make32(seed))) }

func TestRebindingRepointsTheEpisodeAndConsumesOneRebind(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		must(t, tx.RebindEpisode(ctx, "e1", 4, make32(9), []byte(`{"fresh":true}`)))
		if got, want := episodeState(t, ctx, raw), `admitted|4|1|-|-|{"fresh":true}|`+digestHex(9); got != want {
			t.Fatalf("rebound episode = %s, want %s", got, want)
		}
	})
}

func TestBindingTheRequestChangesOnlyTheRequest(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		must(t, tx.BindRequest(ctx, "e1", []byte(`{"bound":1}`)))
		if got, want := episodeState(t, ctx, raw), "admitted|1|0|-|-|{\"bound\":1}|"+digestHex(0); got != want {
			t.Fatalf("episode after BindRequest = %s, want %s", got, want)
		}
	})
}

func TestQuarantiningAnInvalidSnapshotAbandonsTheEpisodeAndConsumesOneRebind(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		must(t, tx.AbandonRebind(ctx, "e1", at, []byte(`{"reason":"invalid"}`)))
		if got, want := episodeState(t, ctx, raw), "abandoned|1|1|"+ts(0)+`|{"reason":"invalid"}|{}|`+digestHex(0); got != want {
			t.Fatalf("quarantined episode = %s, want %s", got, want)
		}
	})
}

func TestTerminalOutcomesCloseTheEpisodeWithTheirDocument(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		close func(*store.Tx, context.Context) error
		want  string
	}{
		"abandon": {func(tx *store.Tx, ctx context.Context) error {
			return tx.AbandonEpisode(ctx, "e1", at, []byte(`{"reason":"killed"}`))
		}, "abandoned|1|0|%s" + `|{"reason":"killed"}|{}|` + "%s"},
		"conclude": {func(tx *store.Tx, ctx context.Context) error {
			return tx.ConcludeEpisode(ctx, "e1", at, []byte(`{"status":"declined"}`))
		}, "concluded|1|0|%s" + `|{"status":"declined"}|{}|` + "%s"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
				admit(t, ctx, tx, "e1")
				must(t, tc.close(tx, ctx))
				if got, want := episodeState(t, ctx, raw), fmt.Sprintf(tc.want, ts(0), digestHex(0)); got != want {
					t.Fatalf("closed episode = %s, want %s", got, want)
				}
			})
		})
	}
}

func TestRetainingAnEpisodeForRetryReopensItWithoutAnOutcome(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		must(t, tx.ConcludeEpisode(ctx, "e1", at, []byte(`{"status":"failed"}`)))
		must(t, tx.RetainEpisodeForRetry(ctx, "e1"))
		if got, want := episodeState(t, ctx, raw), "running|1|0|-|-|{}|"+digestHex(0); got != want {
			t.Fatalf("retained episode = %s, want %s", got, want)
		}
	})
}

func TestAnUnknownEpisodeHasNoLifecycle(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		_, err := tx.EpisodeLifecycle(ctx, "missing")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("EpisodeLifecycle(missing) = %v, want ErrNoRows", err)
		}
		admit(t, ctx, tx, "e1")
		if lifecycle, err := tx.EpisodeLifecycle(ctx, "e1"); err != nil || lifecycle != domain.LifecycleAdmitted {
			t.Fatalf("EpisodeLifecycle = %q err=%v", lifecycle, err)
		}
	})
}

func TestAnEpisodeRecordListsItsAttemptsByFenceAndItsRefusedResults(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		admit(t, ctx, tx, "e1")
		first := startAttempt(t, ctx, tx, "e1", "a-first", 1)
		_, err := tx.FinishAttempt(ctx, first, domain.AttemptAbandoned, at.Add(time.Minute), []byte(`{"reason":"grace_expired"}`))
		must(t, err)
		startAttempt(t, ctx, tx, "e1", "a-second", 2)
		must(t, tx.InsertRejection(ctx, domain.Rejection{ID: "rej_1", EpisodeID: "e1", AttemptID: "a-first", Fence: 1, Reason: domain.RejectStaleAttempt, Details: []byte(`{"late":true}`), At: at.Add(2 * time.Minute)}, true))
		must(t, tx.ConcludeEpisode(ctx, "e1", at.Add(time.Hour), []byte(`{"status":"produced"}`)))

		record, err := tx.Episode(ctx, "t", "e1")
		if err != nil {
			t.Fatal(err)
		}
		if record.EpisodeID != "e1" || record.LifecycleStatus != "concluded" || record.CurrentAttemptID != "a-second" || record.CurrentFence != 2 ||
			record.DispatchPolicy != "shadow" || record.EndedAt != ts(time.Hour) || string(record.Terminal) != `{"status":"produced"}` ||
			record.SnapshotSHA256 != kernel.EncodeDigest(make32(0)) {
			t.Fatalf("episode record = %+v", record)
		}
		if len(record.Attempts) != 2 || record.Attempts[0].AttemptID != "a-first" || record.Attempts[0].Status != "abandoned" ||
			string(record.Attempts[0].Terminal) != `{"reason":"grace_expired"}` || record.Attempts[1].AttemptID != "a-second" || record.Attempts[1].Terminal != nil {
			t.Fatalf("attempts = %+v, want fences 1 then 2 with terminal documents only where recorded", record.Attempts)
		}
		if len(record.Rejections) != 1 || record.Rejections[0].Reason != "stale_attempt" || record.Rejections[0].AttemptID != "a-first" || string(record.Rejections[0].Details) != `{"late":true}` {
			t.Fatalf("rejections = %+v", record.Rejections)
		}
	})
}

func TestAnEpisodeOfAnotherTenantIsNotReadable(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		admit(t, ctx, tx, "e1")
		_, err := tx.Episode(ctx, "other", "e1")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("Episode(other tenant) = %v, want ErrNoRows", err)
		}
	})
}

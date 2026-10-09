package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

func TestTheLedgerReadsAnEpisodesFenceStatusLifecycleAndAdmission(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		admit(t, ctx, tx, "e1")
		identity := beginAttempt(t, ctx, tx, "e1", "a1")
		fence, found, err := ReadEpisodeFence(ctx, tx, "e1")
		if want := (domain.EpisodeFence{TenantID: "t", Lifecycle: domain.LifecycleRunning, Attempt: "a1", HasAttempt: true, Fence: 1}); err != nil || !found || fence != want {
			t.Fatalf("fence = %+v found=%v err=%v, want %+v", fence, found, err, want)
		}
		if _, found, err := ReadEpisodeFence(ctx, tx, "missing"); err != nil || found {
			t.Fatalf("unknown fence found=%v err=%v", found, err)
		}
		if status, err := ReadAttemptStatus(ctx, tx, identity); err != nil || status != domain.AttemptDispatched {
			t.Fatalf("attempt status = %q err=%v", status, err)
		}
		if lifecycle, err := ReadEpisodeLifecycle(ctx, tx, "e1"); err != nil || lifecycle != domain.LifecycleRunning {
			t.Fatalf("lifecycle = %q err=%v", lifecycle, err)
		}
		got, err := ReadAdmission(ctx, tx, "e1")
		if want := admission("e1"); err != nil || got.EpisodeID != want.EpisodeID || got.SituationID != want.SituationID || got.DispatchPolicy != want.DispatchPolicy {
			t.Fatalf("admission = %+v err=%v", got, err)
		}
	})
}

func TestReadingAnAttemptThatDoesNotExistIsRefusedAsTheWrongAttempt(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		admit(t, ctx, tx, "e1")
		beginAttempt(t, ctx, tx, "e1", "a1")
		for name, identity := range map[string]domain.Identity{
			"other attempt": {EpisodeID: "e1", AttemptID: "x", Fence: 1},
			"other fence":   {EpisodeID: "e1", AttemptID: "a1", Fence: 2},
		} {
			if _, err := ReadAttemptStatus(ctx, tx, identity); !domain.IsIdentityReason(err, domain.RejectWrongAttempt) {
				t.Errorf("%s: err = %v, want %s", name, err, domain.RejectWrongAttempt)
			}
		}
	})
}

func TestReadingAnUnknownEpisodeLifecycleOrAdmissionIsAnError(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		if _, err := ReadEpisodeLifecycle(ctx, tx, "missing"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("ReadEpisodeLifecycle = %v, want ErrNoRows", err)
		}
		if _, err := ReadAdmission(ctx, tx, "missing"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("ReadAdmission = %v, want ErrNoRows", err)
		}
	})
}

func TestTheDispatcherSeesTheOldestLiveEpisodeAndSupersededOnesOnlyOnRequest(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		if _, found, err := NextDispatchableEpisode(ctx, tx, "t", false); err != nil || found {
			t.Fatalf("empty ledger: found=%v err=%v", found, err)
		}
		admit(t, ctx, tx, "e1")
		next, found, err := NextDispatchableEpisode(ctx, tx, "t", false)
		if err != nil || !found || next.EpisodeID != "e1" || next.StaleRebindCount != 0 {
			t.Fatalf("next = %+v found=%v err=%v", next, found, err)
		}
		execSQL(t, ctx, raw, "UPDATE episodes SET lifecycle_status = 'superseded', policy_epoch = 'dead' WHERE episode_id = 'e1'")
		execSQL(t, ctx, raw, "INSERT INTO epoch_control (epoch, state, updated_at) VALUES ('dead', 'killed', 'now')")
		if _, found, _ := NextDispatchableEpisode(ctx, tx, "t", false); found {
			t.Fatal("a superseded episode was dispatchable without asking for killed epochs")
		}
		if next, found, err := NextDispatchableEpisode(ctx, tx, "t", true); err != nil || !found || next.EpisodeID != "e1" {
			t.Fatalf("killed-epoch episode = %+v found=%v err=%v", next, found, err)
		}
	})
}

func TestAnOpportunityIsExplainableFromDurableRecordsWhetherCoalescedExpiredOrRefused(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		live := queued{id: "item-e1", situation: "s-e1", expires: 60}
		enqueue(t, ctx, tx, live, queued{id: "item-stale", expires: 5})
		recordAdmittedTrigger(t, ctx, raw, live)
		admit(t, ctx, tx, "e1")
		must(t, MarkSchedulerItemAdmitted(ctx, tx, "item-e1", now))
		identity := beginAttempt(t, ctx, tx, "e1", "a1")
		must(t, RecordRejection(ctx, tx, identity, domain.RejectSnapshotMismatch, []byte(`{"want":2}`), now))
		must(t, ExpireSchedulerItem(ctx, tx, "item-stale", minutes(6)))
		_, err := CoalesceSchedulerItems(ctx, tx, "s-e1", "alarm", minutes(7))
		must(t, err)
		must(t, SupersedeCoalesced(ctx, tx, "s-e1", minutes(7)))

		coalesced, found, err := Scheduling(ctx, tx, "tenant", "trigger-item-e1")
		if err != nil || !found || coalesced.Status != "coalesced" || coalesced.EpisodeID != "e1" || coalesced.EpisodeStatus != "superseded" ||
			len(coalesced.Rejections) != 1 || coalesced.Rejections[0].Reason != "snapshot_mismatch" {
			t.Fatalf("coalesced opportunity = %+v found=%v err=%v", coalesced, found, err)
		}
		expired, found, err := Scheduling(ctx, tx, "tenant", "trigger-item-stale")
		if err != nil || !found || expired.Status != "expired" || expired.EpisodeID != "" || expired.UpdatedAt != kernel.FormatTime(minutes(6)) {
			t.Fatalf("expired opportunity = %+v found=%v err=%v", expired, found, err)
		}
		episode, err := Episode(ctx, tx, "t", "e1")
		if err != nil || episode.LifecycleStatus != "superseded" || len(episode.Attempts) != 1 || episode.Attempts[0].Status != string(domain.AttemptCancelling) || len(episode.Rejections) != 1 {
			t.Fatalf("episode record = %+v err=%v, want a superseded episode with a canceling attempt and its refusal", episode, err)
		}
	})
}

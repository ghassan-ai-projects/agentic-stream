package episodeledger_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func admission(id, situation string) episodeledger.Admission {
	digest := make([]byte, 32)
	return episodeledger.Admission{
		EpisodeID: id, SchedulerItemID: "item-" + id, Kind: episodeledger.KindStandard, TenantID: "tenant", SituationID: situation, SituationVersion: 1,
		ExecutorName: "executor", ExecutorVersion: "v1", ModelPolicy: "policy", PromptVersion: "prompt",
		SnapshotSHA256: digest, PromptSHA256: digest, ObjectiveSHA256: digest, AdmissionKey: append([]byte(id), digest[len(id):]...),
		RequestJSON: []byte(`{}`), DispatchPolicy: "shadow", PolicyEpoch: "epoch-1",
	}
}

func admitEpisode(t *testing.T, db *storage.DB, req episodeledger.Admission) {
	t.Helper()
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error { return episodeledger.Admit(ctx, tx, req, now) })
}

func TestAnEpisodeIsAdmittedRunAndExplainedThroughTheFacade(t *testing.T) {
	t.Parallel()
	db := queueDB(t, "trg-1")
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		item := schedulerItem("item-e1", "trg-1", time.Hour)
		item.SituationID = "sit-1"
		if err := episodeledger.UpsertSchedulerItem(ctx, tx, item, "tenant", make([]byte, 32), now); err != nil {
			return err
		}
		return episodeledger.MarkSchedulerItemAdmitted(ctx, tx, "item-e1", now)
	})
	admitEpisode(t, db, admission("e1", "sit-1"))

	var identity episodeledger.Identity
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		next, found, err := episodeledger.NextDispatchableEpisode(ctx, tx, "tenant", false)
		if err != nil || !found || next.EpisodeID != "e1" {
			return errors.Join(err, errors.New("e1 was not the next dispatchable episode"))
		}
		if identity, err = episodeledger.StartAttemptOwned(ctx, tx, "e1", "a1", "owner-1", ownerHolds, now); err != nil {
			return err
		}
		for _, to := range []episodeledger.AttemptStatus{episodeledger.AttemptRunning, episodeledger.AttemptProduced} {
			if err := episodeledger.TransitionAttempt(ctx, tx, identity, to, now.Add(time.Second), []byte(`{}`), ownerHolds); err != nil {
				return err
			}
		}
		stale := identity
		stale.Fence = 0
		if err := episodeledger.RecordRejection(ctx, tx, stale, episodeledger.RejectStaleAttempt, nil, now.Add(2*time.Second)); err != nil {
			return err
		}
		return episodeledger.Conclude(ctx, tx, "e1", now.Add(3*time.Second), []byte(`{"status":"produced"}`))
	})

	assertEpisodeReads(t, db, identity)
	assertOpportunityIsExplained(t, db)
}

func assertEpisodeReads(t *testing.T, db *storage.DB, identity episodeledger.Identity) {
	t.Helper()
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		fence, found, err := episodeledger.ReadEpisodeFence(ctx, tx, "e1")
		if err != nil || !found || fence.Attempt != "a1" || fence.Fence != 1 || fence.Lifecycle != episodeledger.LifecycleConcluded || !reasonIs(fence.CheckOpenIdentity(identity), episodeledger.RejectEpisodeClosed) {
			t.Errorf("fence = %+v found=%v err=%v, want the concluded episode to refuse its own attempt", fence, found, err)
		}
		if status, err := episodeledger.ReadAttemptStatus(ctx, tx, identity); err != nil || status != episodeledger.AttemptProduced || !episodeledger.IsTerminalAttempt(status) {
			t.Errorf("attempt status = %q err=%v, want produced", status, err)
		}
		if _, found, err := episodeledger.NextDispatchableEpisode(ctx, tx, "tenant", false); err != nil || found {
			t.Errorf("a concluded episode is still dispatchable: found=%v err=%v", found, err)
		}
		return nil
	})
	if lifecycle, err := episodeledger.ReadEpisodeLifecycle(t.Context(), db.DB, "e1"); err != nil || lifecycle != episodeledger.LifecycleConcluded {
		t.Errorf("lifecycle = %q err=%v", lifecycle, err)
	}
	if admission, err := episodeledger.ReadAdmission(t.Context(), db.DB, "e1"); err != nil || admission.SituationID != "sit-1" || admission.DispatchPolicy != "shadow" || admission.PolicyEpoch != "epoch-1" {
		t.Errorf("admission = %+v err=%v", admission, err)
	}
}

func assertOpportunityIsExplained(t *testing.T, db *storage.DB) {
	t.Helper()
	record, found, err := episodeledger.Scheduling(t.Context(), db.DB, "tenant", "trg-1")
	if err != nil || !found || record.Status != "admitted" || record.EpisodeID != "e1" || record.EpisodeStatus != "concluded" ||
		len(record.Rejections) != 1 || record.Rejections[0].Reason != string(episodeledger.RejectStaleAttempt) {
		t.Errorf("scheduling = %+v found=%v err=%v, want the admitted opportunity, its concluded episode and the refusal", record, found, err)
	}
	episode, err := episodeledger.Episode(t.Context(), db.DB, "tenant", "e1")
	if err != nil || episode.LifecycleStatus != "concluded" || len(episode.Attempts) != 1 || episode.Attempts[0].Status != "produced" || len(episode.Rejections) != 1 {
		t.Errorf("episode = %+v err=%v, want one produced attempt and one refusal", episode, err)
	}
	if _, found, err := episodeledger.Scheduling(t.Context(), db.DB, "tenant", "trg-unknown"); err != nil || found {
		t.Errorf("an unknown trigger has scheduling: found=%v err=%v", found, err)
	}
}

func TestAReconsiderationCollidingWithALiveEpisodeReportsTheConflictThroughTheFacade(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	admitEpisode(t, db, admission("e1", "sit-1"))
	collision := admission("e2", "sit-1")
	collision.Kind = episodeledger.KindReconsider
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return episodeledger.Admit(t.Context(), tx, collision, now) })
	if !errors.Is(err, episodeledger.ErrLiveEpisodeConflict) {
		t.Fatalf("Admit = %v, want ErrLiveEpisodeConflict", err)
	}
}

func TestSupersessionCancelsLiveEpisodesOfAKilledEpochAndOfCoalescedItems(t *testing.T) {
	t.Parallel()
	db := queueDB(t)
	killed := admission("e-killed", "sit-killed")
	killed.PolicyEpoch = "dead"
	coalesced := admission("e-coalesced", "sit-coalesced")
	admitEpisode(t, db, killed)
	admitEpisode(t, db, coalesced)
	var killedAttempt, coalescedAttempt episodeledger.Identity
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if killedAttempt, err = episodeledger.StartAttempt(ctx, tx, "e-killed", "a-killed", now); err != nil {
			return err
		}
		if coalescedAttempt, err = episodeledger.StartAttempt(ctx, tx, "e-coalesced", "a-coalesced", now); err != nil {
			return err
		}
		item := schedulerItem("item-e-coalesced", "trg-x", time.Hour)
		item.SituationID = "sit-coalesced"
		if err := episodeledger.UpsertSchedulerItem(ctx, tx, item, "tenant", make([]byte, 32), now); err != nil {
			return err
		}
		return episodeledger.CoalesceSkippedItem(ctx, tx, "item-e-coalesced", now)
	})
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		return errors.Join(episodeledger.SupersedeEpoch(ctx, tx, "dead", now), episodeledger.SupersedeCoalesced(ctx, tx, "sit-coalesced", now))
	})
	for _, tc := range []struct {
		episode  string
		identity episodeledger.Identity
	}{{"e-killed", killedAttempt}, {"e-coalesced", coalescedAttempt}} {
		if lifecycle, err := episodeledger.ReadEpisodeLifecycle(t.Context(), db.DB, tc.episode); err != nil || lifecycle != episodeledger.LifecycleSuperseded {
			t.Errorf("%s lifecycle = %q err=%v, want superseded", tc.episode, lifecycle, err)
		}
		inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
			if err := episodeledger.TransitionAttempt(ctx, tx, tc.identity, episodeledger.AttemptProduced, now, nil, nil); !reasonIs(err, episodeledger.RejectEpisodeClosed) {
				t.Errorf("%s output after supersession = %v, want episode_closed", tc.episode, err)
			}
			return nil
		})
	}
}

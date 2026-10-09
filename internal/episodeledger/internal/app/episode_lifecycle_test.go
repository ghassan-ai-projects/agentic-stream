package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func TestAnEpisodeIsReboundBoundAndConcludedThroughItsLifecycleWriters(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		must(t, Rebind(ctx, tx, "e1", 2, make([]byte, 32), []byte(`{"snapshot":"fresh"}`)))
		must(t, BindRequest(ctx, tx, "e1", []byte(`{"attempt":"bound"}`)))
		state := queryText(t, ctx, raw, "SELECT situation_version || '|' || stale_rebind_count || '|' || CAST(request_json AS TEXT) FROM episodes WHERE episode_id = 'e1'")
		if want := `2|1|{"attempt":"bound"}`; state != want {
			t.Fatalf("episode = %s, want %s", state, want)
		}
		must(t, Conclude(ctx, tx, "e1", now, []byte(`{"status":"declined"}`)))
		if got := lifecycleOf(t, ctx, raw, "e1"); got != "concluded" {
			t.Fatalf("lifecycle = %s, want concluded", got)
		}
		must(t, RetainForRetry(ctx, tx, "e1"))
		if got := lifecycleOf(t, ctx, raw, "e1"); got != "running" {
			t.Fatalf("lifecycle after retention = %s, want running", got)
		}
	})
}

func TestAnEpisodeIsAbandonedWithItsReasonOrQuarantinedWithAnInvalidSnapshot(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e-abandoned")
		admit(t, ctx, tx, "e-quarantined")
		must(t, Abandon(ctx, tx, "e-abandoned", now, []byte(`{"reason":"killed"}`)))
		must(t, AbandonRebind(ctx, tx, "e-quarantined", now, []byte(`{"reason":"invalid_snapshot"}`)))
		for id, want := range map[string]string{
			"e-abandoned":   `abandoned|0|{"reason":"killed"}`,
			"e-quarantined": `abandoned|1|{"reason":"invalid_snapshot"}`,
		} {
			got := queryText(t, ctx, raw, "SELECT lifecycle_status || '|' || stale_rebind_count || '|' || CAST(terminal_json AS TEXT) FROM episodes WHERE episode_id = ?", id)
			if got != want {
				t.Errorf("%s = %s, want %s", id, got, want)
			}
		}
	})
}

func TestAKilledEpochSupersedesItsLiveEpisodesSoTheirAttemptsCanOnlyBeCancelled(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		doomed := admission("e-doomed")
		doomed.PolicyEpoch = "dead"
		must(t, Admit(ctx, tx, doomed, now))
		identity := beginAttempt(t, ctx, tx, "e-doomed", "a1")
		transition(t, ctx, tx, identity, domain.AttemptRunning)
		must(t, SupersedeEpoch(ctx, tx, "dead", now))
		if got := lifecycleOf(t, ctx, raw, "e-doomed"); got != "superseded" {
			t.Fatalf("lifecycle = %s, want superseded", got)
		}
		if err := TransitionAttempt(ctx, tx, identity, domain.AttemptProduced, now, nil, nil); !domain.IsIdentityReason(err, domain.RejectEpisodeClosed) {
			t.Fatalf("output of a killed epoch = %v, want %s", err, domain.RejectEpisodeClosed)
		}
		transition(t, ctx, tx, identity, domain.AttemptCancelled)
	})
}

func TestCoalescingSupersedesTheEpisodeAndAsksItsInFlightAttemptToCancel(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		item := queued{id: "item-e1", situation: "s-e1", expires: 60}
		enqueue(t, ctx, tx, item)
		recordAdmittedTrigger(t, ctx, raw, item)
		admit(t, ctx, tx, "e1")
		must(t, MarkSchedulerItemAdmitted(ctx, tx, "item-e1", now))
		identity := beginAttempt(t, ctx, tx, "e1", "a1")
		transition(t, ctx, tx, identity, domain.AttemptRunning)
		coalesced, err := CoalesceSchedulerItems(ctx, tx, "s-e1", "alarm", now)
		if err != nil || len(coalesced) != 1 {
			t.Fatalf("coalesced = %v err=%v", coalesced, err)
		}
		must(t, SupersedeCoalesced(ctx, tx, "s-e1", now))
		if got := lifecycleOf(t, ctx, raw, "e1") + "|" + attemptStatus(t, ctx, raw, "a1"); got != "superseded|"+string(domain.AttemptCancelling) {
			t.Fatalf("state = %s, want a superseded episode whose attempt is canceling", got)
		}
		transition(t, ctx, tx, identity, domain.AttemptCancelled)
	})
}

func TestSupersedingASituationWithNoCoalescedItemChangesNothing(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		identity := beginAttempt(t, ctx, tx, "e1", "a1")
		must(t, SupersedeCoalesced(ctx, tx, "s-e1", now))
		if got := lifecycleOf(t, ctx, raw, "e1") + "|" + attemptStatus(t, ctx, raw, identity.AttemptID); got != "running|dispatched" {
			t.Fatalf("state = %s, want the live episode and attempt untouched", got)
		}
	})
}

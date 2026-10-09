package app

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func TestStartingAnAttemptAllocatesTheNextFenceAndMarksTheEpisodeRunning(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		first := beginAttempt(t, ctx, tx, "e1", "a1")
		if first != (domain.Identity{EpisodeID: "e1", AttemptID: "a1", Fence: 1}) {
			t.Fatalf("first identity = %+v, want fence 1 and no owner epoch", first)
		}
		fence, _, err := tx.ReadEpisodeFence(ctx, "e1")
		if want := (domain.EpisodeFence{TenantID: "t", Lifecycle: domain.LifecycleRunning, Attempt: "a1", HasAttempt: true, Fence: 1}); err != nil || fence != want {
			t.Fatalf("episode fence = %+v err=%v, want %+v", fence, err, want)
		}
		transition(t, ctx, tx, first, domain.AttemptRunning)
		must(t, TransitionAttempt(ctx, tx, first, domain.AttemptAbandoned, now, []byte(`{"reason":"grace_expired"}`), nil))
		second := beginAttempt(t, ctx, tx, "e1", "a2")
		if second.Fence != 2 || second.AttemptID != "a2" {
			t.Fatalf("retry identity = %+v, want fence 2", second)
		}
	})
}

func TestAnEpisodeAllowsOnlyOneActiveAttempt(t *testing.T) {
	t.Parallel()
	for _, active := range []domain.AttemptStatus{domain.AttemptDispatched, domain.AttemptRunning, domain.AttemptCancelling} {
		t.Run(string(active), func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
				admit(t, ctx, tx, "e1")
				first := beginAttempt(t, ctx, tx, "e1", "a1")
				execSQL(t, ctx, raw, "UPDATE episode_attempts SET status = ? WHERE attempt_id = 'a1'", active)
				_, err := StartAttempt(ctx, tx, "e1", "a2", now)
				if err == nil || !strings.Contains(err.Error(), "episode e1 already has active attempt a1") {
					t.Fatalf("second attempt while %s = %v, want a refusal naming the active attempt", active, err)
				}
				if rows := queryText(t, ctx, raw, "SELECT COUNT(*) FROM episode_attempts WHERE attempt_id = 'a2'"); rows != "0" {
					t.Fatalf("the refused attempt left %s rows", rows)
				}
				if fence, _, _ := tx.ReadEpisodeFence(ctx, "e1"); fence.Fence != first.Fence || fence.Attempt != "a1" {
					t.Fatalf("the refused attempt moved the episode fence to %+v", fence)
				}
			})
		})
	}
}

func TestAnAttemptStartsOnlyOnAKnownLiveEpisode(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		if _, err := StartAttempt(ctx, tx, "missing", "a1", now); !domain.IsIdentityReason(err, domain.RejectUnknownEpisode) {
			t.Errorf("unknown episode = %v, want %s", err, domain.RejectUnknownEpisode)
		}
		for _, lifecycle := range []domain.LifecycleStatus{domain.LifecycleConcluded, domain.LifecycleClosed, domain.LifecycleSuperseded, domain.LifecycleExpired, domain.LifecycleAbandoned} {
			id := "e-" + string(lifecycle)
			admit(t, ctx, tx, id)
			closeEpisode(t, ctx, raw, id, lifecycle)
			if _, err := StartAttempt(ctx, tx, id, "a-"+id, now); !domain.IsIdentityReason(err, domain.RejectEpisodeClosed) {
				t.Errorf("%s episode = %v, want %s", lifecycle, err, domain.RejectEpisodeClosed)
			}
		}
	})
}

func TestStartingAnAttemptNeedsBothIdentifiersAndAnOwnedStartNeedsAnEpoch(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		admit(t, ctx, tx, "e1")
		if _, err := StartAttemptOwned(ctx, tx, "e1", "a1", "", heldBy("epoch"), now); err == nil || err.Error() != "runtime owner epoch is required" {
			t.Errorf("owned start without an epoch = %v, want it refused", err)
		}
		for _, ids := range [][2]string{{"", "a1"}, {"e1", ""}, {"", ""}} {
			if _, err := StartAttempt(ctx, tx, ids[0], ids[1], now); err == nil || err.Error() != "episode and attempt IDs are required" {
				t.Errorf("StartAttempt(%q, %q) = %v, want the missing identifiers refused", ids[0], ids[1], err)
			}
		}
	})
}

func TestAnAttemptMovesAlongTheTransitionTableAndRefusesTheRest(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		identity := beginAttempt(t, ctx, tx, "e1", "a1")
		err := TransitionAttempt(ctx, tx, identity, domain.AttemptProduced, now, nil, nil)
		if err == nil || err.Error() != "invalid attempt transition dispatched -> produced" {
			t.Fatalf("dispatched -> produced = %v, want it refused by name", err)
		}
		transition(t, ctx, tx, identity, domain.AttemptRunning)
		if err := TransitionAttempt(ctx, tx, identity, domain.AttemptDispatched, now, nil, nil); err == nil || err.Error() != "invalid attempt transition running -> dispatched" {
			t.Fatalf("running -> dispatched = %v, want it refused by name", err)
		}
		if status := attemptStatus(t, ctx, raw, "a1"); status != "running" {
			t.Fatalf("a refused transition moved the attempt to %s", status)
		}
		transition(t, ctx, tx, identity, domain.AttemptProduced)
	})
}

func TestATerminalAttemptAcceptsNoFurtherTransition(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		identity := beginAttempt(t, ctx, tx, "e1", "a1")
		transition(t, ctx, tx, identity, domain.AttemptRunning)
		transition(t, ctx, tx, identity, domain.AttemptDeclined)
		for _, to := range []domain.AttemptStatus{domain.AttemptRunning, domain.AttemptProduced, domain.AttemptFailed, domain.AttemptAbandoned} {
			if err := TransitionAttempt(ctx, tx, identity, to, now, nil, nil); !domain.IsIdentityReason(err, domain.RejectTerminalAttempt) {
				t.Errorf("declined -> %s = %v, want %s", to, err, domain.RejectTerminalAttempt)
			}
		}
		if status := attemptStatus(t, ctx, raw, "a1"); status != "declined" {
			t.Fatalf("attempt status = %s, want declined", status)
		}
	})
}

func TestATransitionRecordsItsTimesAndTerminalDocument(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		identity, err := StartAttempt(ctx, tx, "e1", "a1", now)
		must(t, err)
		must(t, TransitionAttempt(ctx, tx, identity, domain.AttemptRunning, now.Add(1), nil, nil))
		must(t, TransitionAttempt(ctx, tx, identity, domain.AttemptTimedOut, now.Add(2), []byte(`{"after":"grace"}`), nil))
		row := queryText(t, ctx, raw, "SELECT status || '|' || started_at || '|' || ended_at || '|' || CAST(terminal_json AS TEXT) FROM episode_attempts WHERE attempt_id = 'a1'")
		if want := "timed_out|" + ts(0) + "|" + ts(2) + `|{"after":"grace"}`; row != want {
			t.Fatalf("attempt row = %s, want %s", row, want)
		}
	})
}

func TestALateAttemptCannotTransitionAfterARetryStartedANewFence(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		first := beginAttempt(t, ctx, tx, "e1", "a1")
		transition(t, ctx, tx, first, domain.AttemptRunning)
		transition(t, ctx, tx, first, domain.AttemptAbandoned)
		second := beginAttempt(t, ctx, tx, "e1", "a2")
		if err := TransitionAttempt(ctx, tx, first, domain.AttemptProduced, now, nil, nil); !domain.IsIdentityReason(err, domain.RejectStaleAttempt) {
			t.Fatalf("late output of the first attempt = %v, want %s", err, domain.RejectStaleAttempt)
		}
		transition(t, ctx, tx, second, domain.AttemptRunning)
		if status := attemptStatus(t, ctx, raw, "a1"); status != "abandoned" {
			t.Fatalf("late output rewrote the first attempt to %s", status)
		}
	})
}

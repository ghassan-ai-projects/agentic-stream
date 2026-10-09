package app

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func TestRecoveryNeedsATransactionAndACurrentEpoch(t *testing.T) {
	t.Parallel()
	const want = "recovery transaction and current epoch are required"
	if _, err := RecoverUnfinishedAttempts(t.Context(), store.Join(nil), "epoch", now, nil); err == nil || err.Error() != want {
		t.Fatalf("recovery without a transaction = %v, want %q", err, want)
	}
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		if _, err := RecoverUnfinishedAttempts(ctx, tx, "", now, nil); err == nil || err.Error() != want {
			t.Fatalf("recovery without an epoch = %v, want %q", err, want)
		}
	})
}

func TestRecoveryRequeuesAnEpisodeWhoseAttemptBelongedToAnOlderEpoch(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		identity, err := StartAttemptOwned(ctx, tx, "e1", "a1", "old-epoch", heldBy("old-epoch"), now)
		must(t, err)
		must(t, TransitionAttempt(ctx, tx, identity, domain.AttemptRunning, now, nil, heldBy("old-epoch")))
		settler := &recordingSettler{}
		report, err := RecoverUnfinishedAttempts(ctx, tx, "new-epoch", now, settler)
		if want := (domain.RecoveryReport{AbandonedAttempts: 1, RequeuedEpisodes: 1}); err != nil || report != want {
			t.Fatalf("report = %+v err=%v, want %+v", report, err, want)
		}
		if len(settler.episodes) != 0 {
			t.Fatalf("a requeued episode released its cost reservation: %v", settler.episodes)
		}
		row := queryText(t, ctx, raw, `SELECT a.status || '|' || e.lifecycle_status || '|' || e.current_fence || '|' || json_extract(a.terminal_json, '$.reason') || '|' || json_extract(a.terminal_json, '$.previous_owner_epoch')
			FROM episode_attempts a JOIN episodes e ON e.episode_id = a.episode_id WHERE a.attempt_id = 'a1'`)
		if want := "abandoned|running|1|runtime_restart|old-epoch"; row != want {
			t.Fatalf("recovered state = %s, want %s", row, want)
		}
		next, err := StartAttemptOwned(ctx, tx, "e1", "a2", "new-epoch", heldBy("new-epoch"), now)
		if err != nil || next.Fence != 2 {
			t.Fatalf("attempt after recovery = %+v err=%v, want the next fence 2", next, err)
		}
		if err := ValidateWorkerIdentity(ctx, tx, identity, heldBy("old-epoch")); !domain.IsIdentityReason(err, domain.RejectStaleAttempt) {
			t.Fatalf("output of the pre-restart attempt = %v, want %s", err, domain.RejectStaleAttempt)
		}
	})
}

func TestRecoveryAbandonsAnEpisodeWhoseAttemptWasBeingCancelledAndReleasesItsCost(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		identity := beginAttempt(t, ctx, tx, "e1", "a1")
		transition(t, ctx, tx, identity, domain.AttemptCancelling)
		settler := &recordingSettler{}
		report, err := RecoverUnfinishedAttempts(ctx, tx, "new-owner", now, settler)
		if want := (domain.RecoveryReport{AbandonedAttempts: 1, AbandonedEpisodes: 1}); err != nil || report != want {
			t.Fatalf("report = %+v err=%v, want %+v", report, err, want)
		}
		if !slices.Equal(settler.episodes, []string{"e1"}) {
			t.Fatalf("settled episodes = %v, want the abandoned episode", settler.episodes)
		}
		if got := lifecycleOf(t, ctx, raw, "e1") + "|" + attemptStatus(t, ctx, raw, "a1"); got != "abandoned|abandoned" {
			t.Fatalf("recovered state = %s, want an abandoned episode and attempt", got)
		}
		if reason := queryText(t, ctx, raw, "SELECT json_extract(terminal_json, '$.reason') FROM episodes WHERE episode_id = 'e1'"); reason != "runtime_restart" {
			t.Fatalf("episode terminal reason = %q, want runtime_restart", reason)
		}
	})
}

func TestRecoveryWithoutACostSettlerStillAbandonsTheCancellingEpisode(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		transition(t, ctx, tx, beginAttempt(t, ctx, tx, "e1", "a1"), domain.AttemptCancelling)
		report, err := RecoverUnfinishedAttempts(ctx, tx, "new-owner", now, nil)
		if err != nil || report.AbandonedEpisodes != 1 || lifecycleOf(t, ctx, raw, "e1") != "abandoned" {
			t.Fatalf("report = %+v err=%v lifecycle=%s", report, err, lifecycleOf(t, ctx, raw, "e1"))
		}
	})
}

func TestARecoveryCostFailureNamesTheEpisodeAndFailsTheRecovery(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		admit(t, ctx, tx, "e1")
		transition(t, ctx, tx, beginAttempt(t, ctx, tx, "e1", "a1"), domain.AttemptCancelling)
		cause := errors.New("ledger unavailable")
		_, err := RecoverUnfinishedAttempts(ctx, tx, "new-owner", now, &recordingSettler{fail: cause})
		if !errors.Is(err, cause) || err == nil || !strings.Contains(err.Error(), "settle abandoned episode cost e1") || !strings.Contains(err.Error(), "settle cost of episode e1") {
			t.Fatalf("recovery = %v, want the settlement failure wrapped with the episode", err)
		}
	})
}

func TestRecoveryLeavesTheCurrentEpochAndFinishedAttemptsAlone(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		current, err := StartAttemptOwned(ctx, tx, "e1", "a-current", "epoch", heldBy("epoch"), now)
		must(t, err)
		must(t, TransitionAttempt(ctx, tx, current, domain.AttemptRunning, now, nil, heldBy("epoch")))
		admit(t, ctx, tx, "e2")
		done := beginAttempt(t, ctx, tx, "e2", "a-done")
		transition(t, ctx, tx, done, domain.AttemptRunning)
		transition(t, ctx, tx, done, domain.AttemptProduced)
		report, err := RecoverUnfinishedAttempts(ctx, tx, "epoch", now, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := attemptStatus(t, ctx, raw, "a-current") + "|" + attemptStatus(t, ctx, raw, "a-done"); got != "running|produced" {
			t.Fatalf("attempt statuses = %s, want the running current-epoch attempt and the produced attempt untouched", got)
		}
		if report != (domain.RecoveryReport{}) {
			t.Fatalf("report = %+v, want nothing recovered", report)
		}
	})
}

func TestRecoveryIsIdempotent(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		admit(t, ctx, tx, "e1")
		beginAttempt(t, ctx, tx, "e1", "a1")
		first, err := RecoverUnfinishedAttempts(ctx, tx, "new-epoch", now, nil)
		if err != nil || first.AbandonedAttempts != 1 {
			t.Fatalf("first recovery = %+v err=%v", first, err)
		}
		if second, err := RecoverUnfinishedAttempts(ctx, tx, "new-epoch", now.Add(1), nil); err != nil || second != (domain.RecoveryReport{}) {
			t.Fatalf("second recovery = %+v err=%v, want nothing left to recover", second, err)
		}
	})
}

package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func supersededWithRunningAttempt(t *testing.T, ctx context.Context, tx *store.Tx, raw *sql.Tx) domain.Identity {
	t.Helper()
	admit(t, ctx, tx, "e1")
	identity := beginAttempt(t, ctx, tx, "e1", "a1")
	transition(t, ctx, tx, identity, domain.AttemptRunning)
	closeEpisode(t, ctx, raw, "e1", domain.LifecycleSuperseded)
	return identity
}

func TestASupersededEpisodeAcknowledgesCancellationButNoOtherOutcome(t *testing.T) {
	t.Parallel()
	for _, to := range []domain.AttemptStatus{domain.AttemptProduced, domain.AttemptDeclined, domain.AttemptFailed, domain.AttemptTimedOut, domain.AttemptCancelling, domain.AttemptCancelled, domain.AttemptAbandoned} {
		t.Run(string(to), func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
				identity := supersededWithRunningAttempt(t, ctx, tx, raw)
				err := TransitionAttempt(ctx, tx, identity, to, now, []byte("{}"), nil)
				if domain.MayAcknowledgeCancellation(to) {
					if err != nil || attemptStatus(t, ctx, raw, "a1") != string(to) {
						t.Fatalf("acknowledging with %s: err=%v status=%s", to, err, attemptStatus(t, ctx, raw, "a1"))
					}
					return
				}
				if !domain.IsIdentityReason(err, domain.RejectEpisodeClosed) || attemptStatus(t, ctx, raw, "a1") != "running" {
					t.Fatalf("output %s on a superseded episode = %v (status %s), want %s and the attempt untouched", to, err, attemptStatus(t, ctx, raw, "a1"), domain.RejectEpisodeClosed)
				}
			})
		})
	}
}

func TestOnlyTheCurrentAttemptMayAcknowledgeCancellationOfAClosedEpisode(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		current := supersededWithRunningAttempt(t, ctx, tx, raw)
		for name, tc := range map[string]struct {
			identity domain.Identity
			want     domain.RejectionReason
		}{
			"older fence":   {domain.Identity{EpisodeID: "e1", AttemptID: "a0", Fence: current.Fence - 1}, domain.RejectStaleAttempt},
			"other attempt": {domain.Identity{EpisodeID: "e1", AttemptID: "other", Fence: current.Fence}, domain.RejectWrongAttempt},
			"newer fence":   {domain.Identity{EpisodeID: "e1", AttemptID: "a1", Fence: current.Fence + 1}, domain.RejectWrongAttempt},
			"unknown":       {domain.Identity{EpisodeID: "missing", AttemptID: "a1", Fence: 1}, domain.RejectUnknownEpisode},
		} {
			if err := TransitionAttempt(ctx, tx, tc.identity, domain.AttemptCancelled, now, nil, nil); !domain.IsIdentityReason(err, tc.want) {
				t.Errorf("%s: err = %v, want %s", name, err, tc.want)
			}
		}
		if status := attemptStatus(t, ctx, raw, "a1"); status != "running" {
			t.Fatalf("a refused acknowledgement moved the attempt to %s", status)
		}
	})
}

func TestACancelledAttemptCannotAcknowledgeTwice(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		identity := supersededWithRunningAttempt(t, ctx, tx, raw)
		transition(t, ctx, tx, identity, domain.AttemptCancelled)
		if err := TransitionAttempt(ctx, tx, identity, domain.AttemptCancelled, now, nil, nil); !domain.IsIdentityReason(err, domain.RejectTerminalAttempt) {
			t.Fatalf("second acknowledgement = %v, want %s", err, domain.RejectTerminalAttempt)
		}
	})
}

func TestCancellationAcknowledgementOfAClosedEpisodeIsStillFencedByTheOwner(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		owner store.OwnerCheck
		stale bool
	}{
		"owner holds": {owner: heldBy("epoch")},
		"owner lost":  {owner: lostOwner, stale: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
				identity := startOwned(t, ctx, tx)
				must(t, TransitionAttempt(ctx, tx, identity, domain.AttemptRunning, now, nil, heldBy("epoch")))
				closeEpisode(t, ctx, raw, "e1", domain.LifecycleSuperseded)
				err := TransitionAttempt(ctx, tx, identity, domain.AttemptCancelled, now, []byte("{}"), tc.owner)
				if tc.stale != domain.IsIdentityReason(err, domain.RejectStaleAttempt) || (!tc.stale && err != nil) {
					t.Fatalf("acknowledgement = %v, want stale=%v", err, tc.stale)
				}
			})
		})
	}
}

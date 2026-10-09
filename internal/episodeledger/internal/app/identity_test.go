package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func TestAWorkerIdentityIsRefusedWithTheReasonItBreaks(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		prepare  func(t *testing.T, ctx context.Context, tx *store.Tx, raw *sql.Tx, current domain.Identity)
		identity func(current domain.Identity) domain.Identity
		want     domain.RejectionReason
	}{
		"current attempt is accepted": {identity: func(c domain.Identity) domain.Identity { return c }},
		"unknown episode": {
			identity: func(c domain.Identity) domain.Identity { c.EpisodeID = "missing"; return c },
			want:     domain.RejectUnknownEpisode,
		},
		"closed episode": {
			prepare: func(t *testing.T, ctx context.Context, _ *store.Tx, raw *sql.Tx, _ domain.Identity) {
				t.Helper()
				closeEpisode(t, ctx, raw, "e1", domain.LifecycleConcluded)
			},
			identity: func(c domain.Identity) domain.Identity { return c },
			want:     domain.RejectEpisodeClosed,
		},
		"closed beats a stale fence": {
			prepare: func(t *testing.T, ctx context.Context, _ *store.Tx, raw *sql.Tx, _ domain.Identity) {
				t.Helper()
				closeEpisode(t, ctx, raw, "e1", domain.LifecycleAbandoned)
			},
			identity: func(c domain.Identity) domain.Identity { c.Fence = 0; return c },
			want:     domain.RejectEpisodeClosed,
		},
		"older fence": {
			identity: func(c domain.Identity) domain.Identity { c.Fence = 0; return c },
			want:     domain.RejectStaleAttempt,
		},
		"newer fence": {
			identity: func(c domain.Identity) domain.Identity { c.Fence++; return c },
			want:     domain.RejectWrongAttempt,
		},
		"another attempt id": {
			identity: func(c domain.Identity) domain.Identity { c.AttemptID = "other"; return c },
			want:     domain.RejectWrongAttempt,
		},
		"terminal attempt": {
			prepare: func(t *testing.T, ctx context.Context, tx *store.Tx, _ *sql.Tx, current domain.Identity) {
				t.Helper()
				transition(t, ctx, tx, current, domain.AttemptFailed)
			},
			identity: func(c domain.Identity) domain.Identity { return c },
			want:     domain.RejectTerminalAttempt,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
				admit(t, ctx, tx, "e1")
				current := beginAttempt(t, ctx, tx, "e1", "a1")
				if tc.prepare != nil {
					tc.prepare(t, ctx, tx, raw, current)
				}
				err := ValidateWorkerIdentity(ctx, tx, tc.identity(current), nil)
				if tc.want == "" && err != nil || tc.want != "" && !domain.IsIdentityReason(err, tc.want) {
					t.Fatalf("ValidateWorkerIdentity = %v, want %q", err, tc.want)
				}
			})
		})
	}
}

func TestAnOwnedIdentityIsStaleWhenItsAttemptBelongsToAnotherEpoch(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		admit(t, ctx, tx, "e1")
		owned, err := StartAttemptOwned(ctx, tx, "e1", "a1", "epoch", heldBy("epoch"), now)
		must(t, err)
		impostor := owned
		impostor.OwnerEpoch = "other"
		if err := ValidateWorkerIdentity(ctx, tx, impostor, heldBy("other")); !domain.IsIdentityReason(err, domain.RejectStaleAttempt) {
			t.Fatalf("an identity claiming another epoch's attempt = %v, want %s", err, domain.RejectStaleAttempt)
		}
		unowned := owned
		unowned.OwnerEpoch = ""
		if err := ValidateWorkerIdentity(ctx, tx, unowned, nil); err != nil {
			t.Fatalf("an identity that claims no epoch is checked by fence only: %v", err)
		}
	})
}

func TestTheOwnerLeaseIsJudgedBeforeTheAttemptStateAndAfterTheEpisodeFence(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		identity, err := StartAttemptOwned(ctx, tx, "e1", "a1", "epoch", heldBy("epoch"), now)
		must(t, err)
		must(t, TransitionAttempt(ctx, tx, identity, domain.AttemptFailed, now, nil, heldBy("epoch")))
		if err := ValidateWorkerIdentity(ctx, tx, identity, lostOwner); !domain.IsIdentityReason(err, domain.RejectStaleAttempt) {
			t.Errorf("terminal attempt with a lost owner = %v, want the lease judged first (%s)", err, domain.RejectStaleAttempt)
		}
		if err := ValidateWorkerIdentity(ctx, tx, identity, heldBy("epoch")); !domain.IsIdentityReason(err, domain.RejectTerminalAttempt) {
			t.Errorf("terminal attempt with a held lease = %v, want %s", err, domain.RejectTerminalAttempt)
		}
		closeEpisode(t, ctx, raw, "e1", domain.LifecycleConcluded)
		if err := ValidateWorkerIdentity(ctx, tx, identity, lostOwner); !domain.IsIdentityReason(err, domain.RejectEpisodeClosed) {
			t.Errorf("closed episode with a lost owner = %v, want the episode fence judged first (%s)", err, domain.RejectEpisodeClosed)
		}
	})
}

func TestAnEpisodePointingAtAMissingAttemptRowRefusesEveryIdentityAsTheWrongAttempt(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		execSQL(t, ctx, raw, "UPDATE episodes SET lifecycle_status = 'running', current_attempt_id = 'ghost', current_fence = 1 WHERE episode_id = 'e1'")
		ghost := domain.Identity{EpisodeID: "e1", AttemptID: "ghost", Fence: 1}
		if err := ValidateWorkerIdentity(ctx, tx, ghost, nil); !domain.IsIdentityReason(err, domain.RejectWrongAttempt) {
			t.Fatalf("ValidateWorkerIdentity = %v, want %s", err, domain.RejectWrongAttempt)
		}
		if err := TransitionAttempt(ctx, tx, ghost, domain.AttemptRunning, now, nil, nil); !domain.IsIdentityReason(err, domain.RejectWrongAttempt) {
			t.Fatalf("TransitionAttempt = %v, want %s", err, domain.RejectWrongAttempt)
		}
	})
}

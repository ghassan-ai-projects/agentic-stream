package store_test

import (
	"errors"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type runtimeOwners struct {
	db     *storage.DB
	holder *runtimecontrol.RuntimeOwner
	other  *runtimecontrol.RuntimeOwner
}

func claimedRuntime(t *testing.T) runtimeOwners {
	t.Helper()
	db := replayedStore(t)
	owners := runtimeOwners{
		db:     db,
		holder: &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-a", Lease: time.Minute},
		other:  &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-b", Lease: time.Minute},
	}
	if err := owners.holder.Claim(t.Context(), "epoch"); err != nil {
		t.Fatal(err)
	}
	return owners
}

func assertOwnedRefusal(t *testing.T, err error, want ownedOutcome) {
	t.Helper()
	var rejection *episodeledger.IdentityError
	isRejection := errors.As(err, &rejection)
	switch want {
	case ownedAccepted:
		if err != nil {
			t.Fatalf("holder refused: %v", err)
		}
	case ownedStale:
		if !isRejection || rejection.Reason != episodeledger.RejectStaleAttempt {
			t.Fatalf("err=%v, want a stale attempt rejection", err)
		}
	case ownedUnfenced:
		if err == nil || isRejection {
			t.Fatalf("err=%v, want a refusal that is not a worker rejection", err)
		}
	}
}

type ownedOutcome int

const (
	ownedAccepted ownedOutcome = iota
	ownedStale
	ownedUnfenced
)

func TestOwnedAttemptsAreFencedByTheRuntimeOwnerAssertion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		store func(o runtimeOwners) store.Store
		want  ownedOutcome
	}{
		{"the holding instance", func(o runtimeOwners) store.Store { return store.New(o.db).Fenced(o.holder.Assert) }, ownedAccepted},
		{"the same epoch from another instance", func(o runtimeOwners) store.Store { return store.New(o.db).Fenced(o.other.Assert) }, ownedStale},
		{"no runtime owner check", func(o runtimeOwners) store.Store { return store.New(o.db) }, ownedUnfenced},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			owners := claimedRuntime(t)
			episodeID := admittedEpisodeID(t, owners.db)

			err := tc.store(owners).WithTx(t.Context(), func(tx *store.Tx) error {
				_, err := tx.StartAttemptOwned(t.Context(), episodeID, "attempt", "epoch", time.Now().UTC())
				return err
			})

			assertOwnedRefusal(t, err, tc.want)
		})
	}
}

func TestOwnedTransitionIsRefusedForTheSameEpochFromAnotherInstance(t *testing.T) {
	t.Parallel()
	owners := claimedRuntime(t)
	episodeID := admittedEpisodeID(t, owners.db)
	now := time.Now().UTC()
	var identity episodeledger.Identity
	if err := store.New(owners.db).Fenced(owners.holder.Assert).WithTx(t.Context(), func(tx *store.Tx) error {
		var err error
		identity, err = tx.StartAttemptOwned(t.Context(), episodeID, "attempt", "epoch", now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	transition := func(fenced store.Store) error {
		return fenced.WithTx(t.Context(), func(tx *store.Tx) error {
			return tx.TransitionAttempt(t.Context(), identity, episodeledger.AttemptRunning, now, nil)
		})
	}

	assertOwnedRefusal(t, transition(store.New(owners.db).Fenced(owners.other.Assert)), ownedStale)
	assertOwnedRefusal(t, transition(store.New(owners.db)), ownedUnfenced)
	assertOwnedRefusal(t, transition(store.New(owners.db).Fenced(owners.holder.Assert)), ownedAccepted)
}

package store_test

import (
	"errors"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
)

func TestOwnedAttemptsAreFencedByTheRuntimeOwnerAssertion(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	var episodeID string
	if err := db.QueryRowContext(t.Context(), "SELECT episode_id FROM episodes LIMIT 1").Scan(&episodeID); err != nil {
		t.Fatal(err)
	}
	holder := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-a", Lease: time.Minute}
	if err := holder.Claim(t.Context(), "epoch"); err != nil {
		t.Fatal(err)
	}
	sameEpochOtherInstance := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-b", Lease: time.Minute}
	now := time.Now().UTC()
	for name, tc := range map[string]struct {
		store     store.Store
		wantStale bool
		wantOther bool
	}{
		"the holding instance":                 {store: store.New(db).Fenced(holder.Assert)},
		"the same epoch from another instance": {store: store.New(db).Fenced(sameEpochOtherInstance.Assert), wantStale: true},
		"no runtime owner check":               {store: store.New(db), wantOther: true},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.store.WithTx(t.Context(), func(tx *store.Tx) error {
				_, err := tx.StartAttemptOwned(t.Context(), episodeID, "attempt-"+name, "epoch", now)
				return err
			})
			assertOwnedRefusal(t, err, tc.wantStale, tc.wantOther)
		})
	}
}

func assertOwnedRefusal(t *testing.T, err error, wantStale, wantOther bool) {
	t.Helper()
	var rejection *episodeledger.IdentityError
	isRejection := errors.As(err, &rejection)
	switch {
	case wantStale:
		if !isRejection || rejection.Reason != episodeledger.RejectStaleAttempt {
			t.Fatalf("err=%v, want a stale attempt rejection", err)
		}
	case wantOther:
		if err == nil || isRejection {
			t.Fatalf("err=%v, want a refusal that is not a worker rejection", err)
		}
	default:
		if err != nil {
			t.Fatalf("holder refused: %v", err)
		}
	}
}

func TestOwnedTransitionIsRefusedForTheSameEpochFromAnotherInstance(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	var episodeID string
	if err := db.QueryRowContext(t.Context(), "SELECT episode_id FROM episodes LIMIT 1").Scan(&episodeID); err != nil {
		t.Fatal(err)
	}
	holder := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-a", Lease: time.Minute}
	other := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-b", Lease: time.Minute}
	if err := holder.Claim(t.Context(), "epoch"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	var identity episodeledger.Identity
	if err := store.New(db).Fenced(holder.Assert).WithTx(t.Context(), func(tx *store.Tx) error {
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
	assertOwnedRefusal(t, transition(store.New(db).Fenced(other.Assert)), true, false)
	assertOwnedRefusal(t, transition(store.New(db)), false, true)
	assertOwnedRefusal(t, transition(store.New(db).Fenced(holder.Assert)), false, false)
}

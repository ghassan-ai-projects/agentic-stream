package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

var errCheckFailed = errors.New("owner check failed")

func heldBy(owner string) store.OwnerCheck {
	return func(_ context.Context, _ *sql.Tx, epoch string) error {
		if epoch != owner {
			return domain.ErrOwnerLost
		}
		return nil
	}
}

func lostOwner(context.Context, *sql.Tx, string) error { return domain.ErrOwnerLost }

func failingOwner(context.Context, *sql.Tx, string) error { return errCheckFailed }

func startOwned(t *testing.T, ctx context.Context, tx *store.Tx) domain.Identity {
	t.Helper()
	admit(t, ctx, tx, "e1")
	identity, err := StartAttemptOwned(ctx, tx, "e1", "a1", "epoch", heldBy("epoch"), now)
	if err != nil || identity.OwnerEpoch != "epoch" {
		t.Fatalf("owned start identity=%+v err=%v", identity, err)
	}
	return identity
}

func TestOwnedStartIsFencedByTheOwnerCheck(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		owner    store.OwnerCheck
		stale    bool
		checkErr error
	}{
		"owner lost is a stale attempt":                {owner: lostOwner, stale: true},
		"another epoch's check is a stale attempt":     {owner: heldBy("other"), stale: true},
		"a failing check is returned, not a rejection": {owner: failingOwner, checkErr: errCheckFailed},
		"no check refuses the start":                   {owner: nil},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
				admit(t, ctx, tx, "e1")
				_, err := StartAttemptOwned(ctx, tx, "e1", "a1", "epoch", tc.owner, now)
				assertFenceRefusal(t, err, tc.stale, tc.checkErr)
				if _, found, readErr := tx.ReadAttempt(ctx, domain.Identity{EpisodeID: "e1", AttemptID: "a1", Fence: 1, OwnerEpoch: "epoch"}); readErr != nil || found {
					t.Fatalf("a refused start left an attempt: found=%v err=%v", found, readErr)
				}
			})
		})
	}
}

func assertFenceRefusal(t *testing.T, err error, stale bool, checkErr error) {
	t.Helper()
	switch {
	case stale:
		if !domain.IsIdentityReason(err, domain.RejectStaleAttempt) {
			t.Fatalf("err=%v, want a stale attempt", err)
		}
	case checkErr != nil:
		var rejection *domain.IdentityError
		if !errors.Is(err, checkErr) || errors.As(err, &rejection) {
			t.Fatalf("err=%v, want the check failure unchanged", err)
		}
	default:
		var rejection *domain.IdentityError
		if err == nil || errors.As(err, &rejection) {
			t.Fatalf("err=%v, want a refusal that is not a worker rejection", err)
		}
	}
}

func TestOwnedIdentityIsFencedOnEveryTransition(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		owner    store.OwnerCheck
		stale    bool
		checkErr error
		allowed  bool
		unowned  bool
	}{
		"owner holds":        {owner: heldBy("epoch"), allowed: true},
		"owner lost":         {owner: lostOwner, stale: true},
		"another epoch":      {owner: heldBy("other"), stale: true},
		"the check fails":    {owner: failingOwner, checkErr: errCheckFailed},
		"no check supplied":  {owner: nil},
		"unowned identities": {owner: nil, allowed: true, unowned: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
				identity := startOwned(t, ctx, tx)
				if tc.unowned {
					identity.OwnerEpoch = ""
				}
				err := TransitionAttempt(ctx, tx, identity, domain.AttemptRunning, now, nil, tc.owner)
				if tc.allowed {
					if err != nil {
						t.Fatalf("transition = %v", err)
					}
					return
				}
				assertFenceRefusal(t, err, tc.stale, tc.checkErr)
				status, readErr := tx.ReadAttemptStatus(ctx, identity.AttemptID)
				if readErr != nil || status != domain.AttemptDispatched {
					t.Fatalf("a fenced transition moved the attempt to %q (%v)", status, readErr)
				}
			})
		})
	}
}

func TestOwnerCheckReceivesTheIdentitysEpoch(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		identity := startOwned(t, ctx, tx)
		var asked []string
		record := func(_ context.Context, _ *sql.Tx, epoch string) error {
			asked = append(asked, epoch)
			return nil
		}
		if err := ValidateWorkerIdentity(ctx, tx, identity, record); err != nil {
			t.Fatal(err)
		}
		if len(asked) != 1 || asked[0] != "epoch" {
			t.Fatalf("owner check asked about %v, want the identity's epoch once", asked)
		}
		unowned := identity
		unowned.OwnerEpoch = ""
		if err := requireOwnerLease(ctx, tx, record, unowned.OwnerEpoch); err != nil || len(asked) != 1 {
			t.Fatalf("an identity without an owner epoch consulted the check: %v %v", asked, err)
		}
	})
}

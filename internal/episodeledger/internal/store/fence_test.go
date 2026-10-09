package store_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestAnEpisodeFenceReadsItsCurrentAttemptOrNone(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		if _, found, err := tx.ReadEpisodeFence(ctx, "missing"); err != nil || found {
			t.Fatalf("unknown episode: found=%v err=%v", found, err)
		}
		admit(t, ctx, tx, "e1")
		fresh, found, err := tx.ReadEpisodeFence(ctx, "e1")
		if want := (domain.EpisodeFence{TenantID: "t", Lifecycle: domain.LifecycleAdmitted}); err != nil || !found || fresh != want {
			t.Fatalf("fence before any attempt = %+v found=%v err=%v, want %+v", fresh, found, err, want)
		}
		startAttempt(t, ctx, tx, "e1", "a1", 1)
		started, _, err := tx.ReadEpisodeFence(ctx, "e1")
		if want := (domain.EpisodeFence{TenantID: "t", Lifecycle: domain.LifecycleRunning, Attempt: "a1", HasAttempt: true, Fence: 1}); err != nil || started != want {
			t.Fatalf("fence after the first attempt = %+v err=%v, want %+v", started, err, want)
		}
	})
}

func TestAnAttemptIsReadOnlyUnderItsExactIdentity(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		admit(t, ctx, tx, "e1")
		owned := domain.Identity{EpisodeID: "e1", AttemptID: "a1", Fence: 1, OwnerEpoch: "epoch"}
		must(t, tx.InsertAttempt(ctx, owned, at))
		record, found, err := tx.ReadAttempt(ctx, owned)
		if want := (domain.AttemptRecord{Status: domain.AttemptDispatched, OwnerEpoch: "epoch", HasOwnerEpoch: true}); err != nil || !found || record != want {
			t.Fatalf("owned attempt = %+v found=%v err=%v, want %+v", record, found, err, want)
		}
		for name, wrong := range map[string]domain.Identity{
			"other attempt id": {EpisodeID: "e1", AttemptID: "x", Fence: 1},
			"other fence":      {EpisodeID: "e1", AttemptID: "a1", Fence: 2},
			"other episode":    {EpisodeID: "e2", AttemptID: "a1", Fence: 1},
		} {
			if _, found, err := tx.ReadAttempt(ctx, wrong); err != nil || found {
				t.Errorf("%s: found=%v err=%v, want no attempt", name, found, err)
			}
		}
	})
}

func TestAnAttemptStatusIsReadByIdWithItsEpisodeWhenNamed(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		admit(t, ctx, tx, "e1")
		startAttempt(t, ctx, tx, "e1", "a1", 1)
		if status, err := tx.ReadAttemptStatus(ctx, "a1"); err != nil || status != domain.AttemptDispatched {
			t.Fatalf("ReadAttemptStatus = %q err=%v", status, err)
		}
		if status, err := tx.ReadEpisodeAttemptStatus(ctx, "e1", "a1"); err != nil || status != domain.AttemptDispatched {
			t.Fatalf("ReadEpisodeAttemptStatus = %q err=%v", status, err)
		}
		if _, err := tx.ReadAttemptStatus(ctx, "missing"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("ReadAttemptStatus(missing) = %v, want ErrNoRows", err)
		}
		if _, err := tx.ReadEpisodeAttemptStatus(ctx, "other-episode", "a1"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("an attempt of another episode = %v, want ErrNoRows", err)
		}
	})
}

func TestTheOwnerCheckRunsOnTheCallersTransactionAndItsFailureIsWrapped(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		var asked string
		var onTx *sql.Tx
		holds := func(_ context.Context, checkTx *sql.Tx, epoch string) error {
			asked, onTx = epoch, checkTx
			return nil
		}
		if err := tx.AssertOwner(ctx, holds, "epoch-7"); err != nil || asked != "epoch-7" || onTx != raw {
			t.Fatalf("holding owner: err=%v asked=%q onCallersTx=%v", err, asked, onTx == raw)
		}
		lost := errors.New("lease lost")
		err := tx.AssertOwner(ctx, func(context.Context, *sql.Tx, string) error { return lost }, "epoch-7")
		if !errors.Is(err, lost) || err.Error() != "assert runtime owner epoch epoch-7: lease lost" {
			t.Fatalf("failing owner = %v, want the check failure wrapped with the epoch", err)
		}
	})
}

func TestAnOwnedIdentityNeedsBothACheckAndATransaction(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	holds := func(context.Context, *sql.Tx, string) error { return nil }
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		if err := tx.AssertOwner(ctx, nil, "e"); err == nil || err.Error() != "an owned identity requires a runtime owner check on a transaction" {
			t.Errorf("no check on a transaction = %v", err)
		}
	})
	if err := store.Reader(db.DB).AssertOwner(t.Context(), holds, "e"); err == nil {
		t.Error("a database handle without a transaction passed an owner check")
	}
}

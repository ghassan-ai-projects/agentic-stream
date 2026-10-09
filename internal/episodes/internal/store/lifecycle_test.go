package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
)

var lifecycleTime = time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)

func TestAdmittedEpisodeReportsItsLifecycleAndNoFailedAttempts(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	episodeID := admittedEpisodeID(t, db)
	var lifecycle episodeledger.LifecycleStatus
	var failed int
	if err := inTx(t, db, func(tx *store.Tx) error {
		var err error
		if lifecycle, err = store.EpisodeLifecycle(t.Context(), tx, episodeID); err != nil {
			return err
		}
		failed, err = store.CountFailedAttempts(t.Context(), tx, episodeID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if lifecycle != episodeledger.LifecycleAdmitted || failed != 0 {
		t.Fatalf("lifecycle = %q with %d failed attempts, want admitted and 0", lifecycle, failed)
	}
	if store.New(db).EpisodeSupersededNow(t.Context(), episodeID) {
		t.Fatal("an admitted episode reported superseded")
	}
}

func TestSupersededEpisodeIsSeenAsSupersededOutsideATransaction(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	episodeID := admittedEpisodeID(t, db)
	if _, err := db.ExecContext(t.Context(), "UPDATE episodes SET lifecycle_status = 'superseded' WHERE episode_id = ?", episodeID); err != nil {
		t.Fatal(err)
	}

	if !store.New(db).EpisodeSupersededNow(t.Context(), episodeID) {
		t.Fatal("a superseded episode was not reported superseded")
	}
	if store.New(db).EpisodeSupersededNow(t.Context(), "epi-missing") {
		t.Fatal("an unknown episode was reported superseded")
	}
}

func TestAttemptStatusReadsTheFencedAttemptAndRefusesAMissingOne(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	episodeID := admittedEpisodeID(t, db)
	var identity episodeledger.Identity
	var status episodeledger.AttemptStatus
	if err := store.New(db).WithTx(t.Context(), func(tx *store.Tx) error {
		var err error
		if identity, err = tx.StartAttempt(t.Context(), episodeID, "att-1", lifecycleTime); err != nil {
			return err
		}
		status, err = store.AttemptStatus(t.Context(), tx, identity)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if status != episodeledger.AttemptDispatched {
		t.Fatalf("attempt status = %q, want %q", status, episodeledger.AttemptDispatched)
	}
	err := inTx(t, db, func(tx *store.Tx) error {
		_, err := store.AttemptStatus(t.Context(), tx, episodeledger.Identity{EpisodeID: episodeID, AttemptID: "att-missing", Fence: 1})
		return err
	})
	var rejection *episodeledger.IdentityError
	if !errors.As(err, &rejection) || rejection.Reason != episodeledger.RejectWrongAttempt {
		t.Fatalf("missing attempt error = %v, want a wrong_attempt identity rejection", err)
	}
}

func TestLedgerMovesRunThroughTheEpisodeTransaction(t *testing.T) {
	t.Parallel()
	moves := []struct {
		name   string
		move   func(ctx context.Context, tx *store.Tx, episodeID string) error
		check  string
		wantIn string
	}{
		{"rebind consumes one rebind", func(ctx context.Context, tx *store.Tx, episodeID string) error {
			return tx.Rebind(ctx, episodeID, 1, bytesOf(7), []byte(`{"rebound":true}`))
		}, "SELECT stale_rebind_count || ':' || hex(snapshot_sha256) || ':' || CAST(request_json AS TEXT) FROM episodes WHERE episode_id = ?", "1:0707070707070707070707070707070707070707070707070707070707070707:{\"rebound\":true}"},
		{"abandon quarantines", func(ctx context.Context, tx *store.Tx, episodeID string) error {
			return tx.Abandon(ctx, episodeID, lifecycleTime, []byte(`{"reason":"stale_situation"}`))
		}, "SELECT lifecycle_status || ':' || json_extract(terminal_json, '$.reason') FROM episodes WHERE episode_id = ?", "abandoned:stale_situation"},
		{"abandoning a failed rebind also consumes one", func(ctx context.Context, tx *store.Tx, episodeID string) error {
			return tx.AbandonRebind(ctx, episodeID, lifecycleTime, []byte(`{"reason":"rebind_failed"}`))
		}, "SELECT lifecycle_status || ':' || stale_rebind_count FROM episodes WHERE episode_id = ?", "abandoned:1"},
		{"conclude ends the episode", func(ctx context.Context, tx *store.Tx, episodeID string) error {
			return store.ConcludeEpisode(ctx, tx, episodeID, lifecycleTime, []byte(`{"status":"done"}`))
		}, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", "concluded"},
	}
	for _, tc := range moves {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := replayedStore(t)
			episodeID := admittedEpisodeID(t, db)

			if err := store.New(db).WithTx(t.Context(), func(tx *store.Tx) error { return tc.move(t.Context(), tx, episodeID) }); err != nil {
				t.Fatal(err)
			}

			if got := scalar[string](t, db, tc.check, episodeID); got != tc.wantIn {
				t.Fatalf("episode after the move = %q, want %q", got, tc.wantIn)
			}
		})
	}
}

func TestFailedAttemptIsRetainedForRetryOnlyAfterItsTerminalTransition(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	episodeID := admittedEpisodeID(t, db)
	if err := store.New(db).WithTx(t.Context(), func(tx *store.Tx) error {
		identity, err := tx.StartAttempt(t.Context(), episodeID, "att-1", lifecycleTime)
		if err != nil {
			return err
		}
		if err := tx.TransitionAttempt(t.Context(), identity, episodeledger.AttemptRunning, lifecycleTime, nil); err != nil {
			return err
		}
		if err := tx.TransitionAttempt(t.Context(), identity, episodeledger.AttemptFailed, lifecycleTime, []byte(`{}`)); err != nil {
			return err
		}
		return store.RetainForRetry(t.Context(), tx, episodeID)
	}); err != nil {
		t.Fatal(err)
	}

	if got := scalar[string](t, db, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", episodeID); got != "running" {
		t.Fatalf("lifecycle = %q, want running", got)
	}
}

func TestCostIsReservedAndSettledOnTheEpisodeTransaction(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	episodeID := admittedEpisodeID(t, db)
	ledger := &control.CostLedger{}
	if err := store.New(db).WithTx(t.Context(), func(tx *store.Tx) error {
		if err := tx.ReserveCost(t.Context(), ledger, episodeID, "default", 40, lifecycleTime); err != nil {
			return err
		}
		return tx.SettleCost(t.Context(), ledger, episodeID, 12, lifecycleTime)
	}); err != nil {
		t.Fatal(err)
	}

	var status string
	var reserved, actual int
	if err := db.QueryRowContext(t.Context(), "SELECT status, reserved_micro, actual_micro FROM cost_reservations WHERE episode_id = ?", episodeID).Scan(&status, &reserved, &actual); err != nil {
		t.Fatal(err)
	}
	if status != "settled" || reserved != 40 || actual != 12 {
		t.Fatalf("reservation = %q reserved %d actual %d, want settled 40 12", status, reserved, actual)
	}
	err := store.New(db).WithTx(t.Context(), func(tx *store.Tx) error {
		return tx.SettleCost(t.Context(), ledger, "epi-unreserved", 1, lifecycleTime)
	})
	if err == nil {
		t.Fatal("settling an episode that reserved nothing succeeded")
	}
}

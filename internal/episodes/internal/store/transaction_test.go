package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestJoinedTransactionKeepsCallerOwnership(t *testing.T) {
	t.Parallel()
	db, err := storagetest.Open(t.Context(), filepath.Join(t.TempDir(), "join.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	rollback := errors.New("rollback entire admission")
	err = db.WithTx(t.Context(), func(original *sql.Tx) error {
		joined := store.Join(original)
		check := func(ctx context.Context, received *sql.Tx, epoch string) error {
			if received != original || epoch != "epoch" {
				t.Fatal("epoch check lost caller transaction")
			}
			_, insertErr := received.ExecContext(ctx, `INSERT INTO epoch_control(epoch,state,updated_at) VALUES ('epoch','draining','2026-10-05T00:00:00Z')`)
			return insertErr
		}
		if err := joined.AssertDecisionEpoch(t.Context(), check, "epoch"); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback error = %v", err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM epoch_control WHERE epoch='epoch'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("joined work escaped rollback: %d", count)
	}
}

func TestLifecycleHandoffRollsBackWithDecisionUnit(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	var episodeID string
	if err := db.QueryRowContext(t.Context(), "SELECT episode_id FROM episodes LIMIT 1").Scan(&episodeID); err != nil {
		t.Fatal(err)
	}
	rejected := errors.New("decision persistence rejected")
	err := store.New(db).WithTx(t.Context(), func(tx *store.Tx) error {
		identity, err := tx.StartAttempt(t.Context(), episodeID, "rollback-attempt", time.Unix(0, 0))
		if err != nil {
			return err
		}
		if err := tx.TransitionAttempt(t.Context(), identity, episodeledger.AttemptRunning, time.Unix(1, 0), nil); err != nil {
			return err
		}
		if err := tx.BindRequest(t.Context(), episodeID, []byte(`{"attempt_id":"rollback-attempt"}`)); err != nil {
			return err
		}
		if err := tx.RecordRejection(t.Context(), identity, episodeledger.RejectWrongAttempt, []byte(`{}`), time.Unix(2, 0)); err != nil {
			return err
		}
		if err := tx.TransitionAttempt(t.Context(), identity, episodeledger.AttemptFailed, time.Unix(3, 0), []byte(`{}`)); err != nil {
			return err
		}
		if err := tx.RetainForRetry(t.Context(), episodeID); err != nil {
			return err
		}
		if err := tx.Conclude(t.Context(), episodeID, "2026-10-05T00:00:00Z", []byte(`{}`)); err != nil {
			return err
		}
		return rejected
	})
	if !errors.Is(err, rejected) {
		t.Fatalf("rollback = %v", err)
	}
	var lifecycle string
	if err := db.QueryRowContext(t.Context(), "SELECT lifecycle_status FROM episodes WHERE episode_id=?", episodeID).Scan(&lifecycle); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "admitted" {
		t.Fatalf("lifecycle escaped rollback: %s", lifecycle)
	}
	var attempts int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM episode_attempts WHERE episode_id=?", episodeID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Fatalf("attempts escaped rollback: %d", attempts)
	}
}

func TestProjectionErrorsPreserveCancellation(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := db.WithTx(t.Context(), func(original *sql.Tx) error {
		tx := store.Join(original)
		operations := []func() error{
			func() error { _, err := store.LoadEvaluation(ctx, tx, "trigger"); return err },
			func() error { _, _, _, _, err := store.LoadSnapshot(ctx, tx, "sit", 1); return err },
			func() error { _, err := store.LiveSituationVersion(ctx, tx, "tenant", "sit"); return err },
			func() error { _, err := store.EpisodeLifecycle(ctx, tx, "epi"); return err },
			func() error { _, err := store.CountFailedAttempts(ctx, tx, "epi"); return err },
			func() error { return store.AnnotateRejectedDecision(ctx, tx, "dec", "schema_invalid") },
			func() error { return store.AcceptDecision(ctx, tx, "dec") },
			func() error { return store.InsertDecision(ctx, tx, store.DecisionInsert{EpisodeID: "epi"}) },
			func() error {
				return store.InsertValidatedIntent(ctx, tx, store.ValidatedIntentInsert{Intent: intentFixture()})
			},
			func() error { _, err := store.LoadReconsideration(ctx, tx, store.SchedulerItem{}, 1, ""); return err },
		}
		for i, operation := range operations {
			if err := operation(); !errors.Is(err, context.Canceled) {
				t.Fatalf("operation %d lost cancellation: %v", i, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestShadowDecisionSharesCallerTransactionAndNeverCreatesActionRecords(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, err := storagetest.Open(ctx, filepath.Join(t.TempDir(), "shadow.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	digest := make([]byte, 32)
	decision := domain.ShadowDecision{
		ShadowDecisionID: "shadow", EpisodeID: "episode", DecisionID: "decision", AttemptID: "attempt",
		DecisionJSON: []byte(`{}`), DecisionSHA256: digest, ShadowScore: domain.ShadowWouldApprove,
		TenantID: "tenant", SituationID: "situation", SituationVersion: 1, PolicyEpoch: "epoch",
	}
	rollback := errors.New("caller failed after recording evidence")
	if err := db.WithTx(ctx, func(raw *sql.Tx) error {
		if err := store.Join(raw).RecordShadowDecision(ctx, decision, "now"); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("record/rollback: %v", err)
	}
	if err := db.WithTx(ctx, func(raw *sql.Tx) error { return store.Join(raw).RecordShadowDecision(ctx, decision, "now") }); err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string]int{"shadow_decisions": 1, "intents": 0, "commands": 0, "outbox": 0} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != want {
			t.Fatalf("%s: count=%d want=%d err=%v", table, count, want, err)
		}
	}
}
